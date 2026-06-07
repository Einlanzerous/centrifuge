// Command centrifuge runs the newsletter-curation backend.
//
// Usage:
//
//	centrifuge           # start the HTTP server
//	centrifuge migrate   # apply database migrations, then exit
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/config"
	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/Einlanzerous/centrifuge/internal/httpapi"
	"github.com/Einlanzerous/centrifuge/internal/ingest"
	applog "github.com/Einlanzerous/centrifuge/internal/log"
	"github.com/Einlanzerous/centrifuge/internal/worker"
)

// migrationsDir is the directory holding versioned SQL migrations.
const migrationsDir = "migrations"

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Logger not yet built; emit via the default slog logger.
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	logger := applog.New(cfg.LogLevel)
	slog.SetDefault(logger)

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrate(cfg, logger)
		return
	}

	if err := runServer(cfg, logger); err != nil {
		logger.Error("server", "error", err)
		os.Exit(1)
	}
}

func runMigrate(cfg *config.Config, logger *slog.Logger) {
	ctx := context.Background()
	logger.Info("applying migrations", "dir", migrationsDir)
	if err := db.Migrate(ctx, cfg.DatabaseURL, migrationsDir); err != nil {
		logger.Error("migrate", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")
}

func runServer(cfg *config.Config, logger *slog.Logger) error {
	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	ingestor := ingest.NewIngestor(pool, ingest.WithMaxBodyChars(cfg.IngestMaxChars))
	srv := httpapi.NewServer(cfg, logger, ingestor)

	// The scoring worker runs decoupled from the HTTP path. Its lifecycle is
	// tied to workerCtx, which is cancelled on shutdown so the loop drains.
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	var workerWG sync.WaitGroup
	if cfg.ScoringEnabled {
		scorer := ai.NewScorer(
			ai.NewClient(cfg.OllamaURL, cfg.OllamaModel,
				ai.WithTimeout(cfg.OllamaTimeout),
				ai.WithMaxRetries(cfg.OllamaMaxRetries),
			),
			cfg.RelevanceTopics,
		)
		w := worker.New(pool, scorer,
			worker.WithInterval(cfg.ScoringInterval),
			worker.WithBatchSize(cfg.ScoringBatch),
			worker.WithLogger(logger),
		)
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			w.Run(workerCtx)
		}()
	} else {
		logger.Info("scoring worker disabled (SCORING_ENABLED=false)")
	}

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           srv.Handler(),
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		logger.Info("shutting down", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	// Stop the worker and wait for the in-flight batch to unwind.
	stopWorker()
	workerWG.Wait()

	logger.Info("stopped")
	return nil
}

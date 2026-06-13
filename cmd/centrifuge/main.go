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

	"github.com/Einlanzerous/centrifuge"
	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/config"
	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/Einlanzerous/centrifuge/internal/httpapi"
	"github.com/Einlanzerous/centrifuge/internal/ingest"
	applog "github.com/Einlanzerous/centrifuge/internal/log"
	"github.com/Einlanzerous/centrifuge/internal/mailfeed"
	"github.com/Einlanzerous/centrifuge/internal/worker"
)

// migrationsDir is the path, within the embedded filesystem, holding the
// versioned SQL migrations.
const migrationsDir = "migrations"

func main() {
	// authorize-gmail mints an OAuth refresh token and is a standalone utility —
	// it needs no DATABASE_URL, so handle it before config.Load() (which requires
	// one). It reads the two GMAIL_* credentials directly from the environment.
	if len(os.Args) > 1 && os.Args[1] == "authorize-gmail" {
		if err := runAuthorizeGmail(); err != nil {
			slog.Error("authorize-gmail", "error", err)
			os.Exit(1)
		}
		return
	}

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
	if err := db.Migrate(ctx, cfg.DatabaseURL, centrifuge.MigrationsFS, migrationsDir); err != nil {
		logger.Error("migrate", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")
}

// runAuthorizeGmail runs the one-time OAuth2 consent flow and prints a refresh
// token to store in GMAIL_REFRESH_TOKEN. It reads GMAIL_CLIENT_ID and
// GMAIL_CLIENT_SECRET straight from the environment so it needs no full config.
func runAuthorizeGmail() error {
	clientID, clientSecret := os.Getenv("GMAIL_CLIENT_ID"), os.Getenv("GMAIL_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("set GMAIL_CLIENT_ID and GMAIL_CLIENT_SECRET before running authorize-gmail")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	token, err := mailfeed.Authorize(ctx, clientID, clientSecret)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Success. Set this in the environment (and PROD_ENV_FILE for prod):")
	fmt.Println()
	fmt.Println("  GMAIL_REFRESH_TOKEN=" + token)
	return nil
}

func runServer(cfg *config.Config, logger *slog.Logger) error {
	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	ingestor := ingest.NewIngestor(pool, ingest.WithMaxBodyChars(cfg.IngestMaxChars))
	srv := httpapi.NewServer(cfg, logger, ingestor, pool)

	// The scoring worker runs decoupled from the HTTP path. Its lifecycle is
	// tied to workerCtx, which is cancelled on shutdown so the loop drains.
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	var workerWG sync.WaitGroup
	if cfg.ScoringEnabled {
		// temperature 0 (greedy) prevents repetition spirals that blow the
		// num_predict cap with zero salvageable items (CTFG-43).
		scoreOpts := map[string]any{"temperature": cfg.OllamaTemperature}
		if cfg.OllamaNumPredict > 0 {
			scoreOpts["num_predict"] = cfg.OllamaNumPredict // cap output → runaway truncates fast (CTFG-42)
		}
		scorer := ai.NewScorer(
			ai.NewClient(cfg.OllamaURL, cfg.OllamaModel,
				ai.WithTimeout(cfg.OllamaTimeout),
				ai.WithMaxRetries(cfg.OllamaMaxRetries),
			),
			cfg.RelevanceTopics,
			ai.WithGenerateOptions(scoreOpts),
		)
		w := worker.New(pool, scorer,
			worker.WithInterval(cfg.ScoringInterval),
			worker.WithBatchSize(cfg.ScoringBatch),
			worker.WithMaxScoringAttempts(cfg.ScoringMaxAttempts),
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

	// The Gmail auto-feed (CTFG-24) polls outbound for new mail and feeds it
	// through the same ingestor as /ingest. It shares the worker's lifecycle so
	// it drains on shutdown. Off unless MAILFEED_ENABLED (it needs OAuth creds).
	if cfg.MailfeedEnabled {
		client, err := mailfeed.NewGmailClient(workerCtx, mailfeed.OAuthConfig{
			ClientID:     cfg.GmailClientID,
			ClientSecret: cfg.GmailClientSecret,
			RefreshToken: cfg.GmailRefreshToken,
			User:         cfg.GmailUser,
			Query:        cfg.MailfeedQuery,
			Label:        cfg.MailfeedLabel,
		})
		if err != nil {
			return fmt.Errorf("init gmail feed: %w", err)
		}
		p := mailfeed.New(ingestor, client,
			mailfeed.WithInterval(cfg.MailfeedInterval),
			mailfeed.WithBatch(cfg.MailfeedBatch),
			mailfeed.WithLogger(logger),
		)
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			p.Run(workerCtx)
		}()
	} else {
		logger.Info("mail feed disabled (MAILFEED_ENABLED=false)")
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

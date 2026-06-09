// Package centrifuge embeds assets that must travel with the compiled binary.
package centrifuge

import "embed"

// MigrationsFS holds the versioned SQL migrations applied at startup. Embedding
// them keeps the migrations inside the binary so there is no runtime dependency
// on the source tree being present — the container image ships only the binary,
// not the migrations/ directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

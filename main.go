package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/8op-org/gl1tch/cmd"
	"github.com/8op-org/gl1tch/internal/bootstrap"
	"github.com/8op-org/gl1tch/internal/telemetry"
)

// Build-time variables injected by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Load .env early so all code paths see the vars.
	if home, err := os.UserHomeDir(); err == nil {
		bootstrap.LoadDotenv(filepath.Join(home, ".config", "glitch", ".env"))
	}
	bootstrap.LoadDotenv(".env")

	ctx := context.Background()
	shutdown, err := telemetry.Setup(ctx, "gl1tch")
	if err == nil {
		defer shutdown(ctx) //nolint:errcheck
	}

	if len(os.Args) > 1 {
		if os.Args[1] == "--version" || os.Args[1] == "-v" {
			fmt.Printf("glitch %s (commit %s, built %s)\n", version, commit, date)
			return
		}
	}

	cmd.Execute()
}

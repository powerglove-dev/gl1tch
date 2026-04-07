package glitchd

import (
	"context"

	"github.com/8op-org/gl1tch/internal/telemetry"
)

// SetupTelemetry initializes the OpenTelemetry tracer + meter providers
// and wires the elasticsearch span exporter. Returns a shutdown func the
// caller must defer so spans drain to ES before the process exits.
//
// Exported from pkg/glitchd because glitch-desktop is a separate module
// (module path "glitch-desktop") and Go's internal-package rules forbid
// it from importing github.com/8op-org/gl1tch/internal/telemetry
// directly. Every "headless" entry point (cmd/serve, tests) already
// calls telemetry.Setup; this wrapper is the desktop's path to the
// same wiring without reaching across the internal boundary.
//
// Call this AFTER InstallLogTee so the "telemetry: elasticsearch trace
// exporter enabled" slog line gets shipped to glitch-logs too, and
// BEFORE any code path that opens tracing spans (collector pods, brain,
// pipeline runner) so the first span doesn't fall on the floor.
func SetupTelemetry(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	return telemetry.Setup(ctx, serviceName)
}

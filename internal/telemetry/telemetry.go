package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// feedCh is the package-level channel used by the FeedExporter.
// Created in Setup() with capacity 256.
var feedCh chan FeedSpanEvent

// FeedEvents returns the read-only channel of span events for the TUI feed.
// Must be called after Setup().
func FeedEvents() <-chan FeedSpanEvent {
	return feedCh
}

// tracesFilePath returns the path for the traces JSONL file.
// Uses $XDG_DATA_HOME if set, otherwise falls back to $HOME/.local/share.
func tracesFilePath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "glitch", "traces.jsonl")
}

// newFileExporter opens (or creates/appends) the traces file and returns
// an stdouttrace exporter writing to it, plus the file as a Closer.
func newFileExporter(path string) (sdktrace.SpanExporter, io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("telemetry: create traces dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: open traces file: %w", err)
	}
	exp, err := stdouttrace.New(stdouttrace.WithWriter(f), stdouttrace.WithPrettyPrint())
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("telemetry: create file trace exporter: %w", err)
	}
	return exp, f, nil
}

// Setup initialises the global OTel TracerProvider and MeterProvider.
// If OTEL_EXPORTER_OTLP_ENDPOINT is set, traces are exported via OTLP gRPC;
// otherwise they go to a file at the XDG data home. Metrics always go to stdout.
// The returned shutdown func must be called before the process exits.
func Setup(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	// We use NewSchemaless instead of Merge(resource.Default(), ...)
	// because resource.Default() adopts whatever schema URL is baked
	// into the currently-linked OTel SDK version, and merging two
	// resources with different schema URLs returns a conflict error.
	// glitch-desktop and gl1tch-cli link different SDK minor versions
	// via their separate go.mod files, so a hardcoded semconv/vX.Y
	// import here clashes with the desktop's default resource every
	// time the SDK is bumped. NewSchemaless skips the schema URL
	// entirely — we still get the service.name attribute Kibana cares
	// about, and we sidestep the version-lockstep requirement.
	res := resource.NewSchemaless(
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion("dev"),
	)

	// Create the feed channel.
	feedCh = make(chan FeedSpanEvent, 256)
	feedExp := NewFeedExporter(feedCh)

	var traceOpts []sdktrace.TracerProviderOption
	traceOpts = append(traceOpts, sdktrace.WithResource(res))
	traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.AlwaysSample()))

	// Wire the feed exporter as a simple span processor (immediate delivery).
	traceOpts = append(traceOpts, sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(feedExp)))

	var fileCloser io.Closer
	// The file exporter is always wired as a local backstop — traces
	// land in $XDG_DATA_HOME/glitch/traces.jsonl so you can grep a
	// specific run even when ES and APM are both down. It's cheap
	// (append-only) and survives the full range of infra failures.
	tracesPath := tracesFilePath()
	if fileExp, closer, err := newFileExporter(tracesPath); err == nil {
		fileCloser = closer
		traceOpts = append(traceOpts, sdktrace.WithBatcher(fileExp))
	}

	// Generic OTLP exporter (non-APM path). Honored for users who run
	// their own OTel collector and set OTEL_EXPORTER_OTLP_ENDPOINT to
	// point at it. Kept separate from the APM path below so you can
	// ship to both a custom collector and apm-server at the same time.
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		otlpExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
		if err != nil {
			slog.Warn("telemetry: otlp trace exporter disabled", "err", err)
		} else {
			traceOpts = append(traceOpts, sdktrace.WithBatcher(otlpExp))
			slog.Info("telemetry: otlp trace exporter enabled", "endpoint", endpoint)
		}
	}

	// Elastic APM exporter — OTLP/gRPC to apm-server:8200 by default.
	// apm-server normalizes our OTel spans into traces-apm-* docs so
	// Kibana's APM UI (Services, Transactions, Errors) lights up on
	// top of the exact same data our custom ES exporter writes to
	// glitch-traces. Two destinations, two audiences, both useful.
	//
	// Default endpoint is localhost:8200 so the docker-compose stack
	// "just works" for local dev. Set GL1TCH_APM_DISABLE=1 to skip
	// the APM exporter entirely; set GL1TCH_APM_ENDPOINT=host:port to
	// point at a non-default apm-server. Failures here are always
	// warnings — APM is an enhancement, never a hard dependency.
	if os.Getenv("GL1TCH_APM_DISABLE") != "1" {
		apmEndpoint := os.Getenv("GL1TCH_APM_ENDPOINT")
		if apmEndpoint == "" {
			apmEndpoint = "localhost:8200"
		}
		apmExp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(apmEndpoint),
		)
		if err != nil {
			slog.Warn("telemetry: apm trace exporter disabled", "err", err, "endpoint", apmEndpoint)
		} else {
			traceOpts = append(traceOpts, sdktrace.WithBatcher(apmExp))
			slog.Info("telemetry: apm trace exporter enabled", "endpoint", apmEndpoint)
		}

		// Wire the APM error sink in parallel with the span exporter.
		// The sink uses HTTP to apm-server's native intake endpoint
		// (not OTLP) because error events have a much richer schema
		// than OTel span events — grouping_key, culprit, exception,
		// stacktrace, log.level, trace.id correlation. Going direct
		// to the intake endpoint is the cheapest way to get the full
		// Errors UI without pulling in go.elastic.co/apm alongside
		// the OTel SDK. See apm_error.go for the schema rationale.
		//
		// Endpoint for the sink is http://host:port (the intake path
		// is appended inside newAPMErrorSink), so we prefix "http://"
		// when the user-provided endpoint is bare "host:port" (the
		// OTLP form). An explicit scheme in GL1TCH_APM_ENDPOINT is
		// passed through unchanged for users on TLS apm-servers.
		intakeEndpoint := apmEndpoint
		if !strings.Contains(intakeEndpoint, "://") {
			intakeEndpoint = "http://" + intakeEndpoint
		}
		installAPMErrorSink(newAPMErrorSink(intakeEndpoint, serviceName))
		slog.Info("telemetry: apm error sink enabled", "endpoint", intakeEndpoint)
	}

	// Elasticsearch exporter — always wired in addition to whichever
	// of OTLP/file is configured above. We want EVERY span to land
	// in glitch-traces so the brain popover, Kibana Discover, and
	// any future "what just broke" query has the same source of
	// truth as the file exporter (the file is the local backstop;
	// ES is the queryable history).
	//
	// ES address comes from GL1TCH_ES_ADDRESS (or defaults to
	// http://localhost:9200). We intentionally do NOT read the
	// collector's observer.yaml from here — that would create an
	// internal/telemetry → internal/collector import dependency,
	// and since internal/collector now imports internal/telemetry
	// for CaptureError the cycle is unbreakable. Env var + default
	// is the right boundary for a base infrastructure package.
	esAddr := os.Getenv("GL1TCH_ES_ADDRESS")
	if esAddr == "" {
		esAddr = "http://localhost:9200"
	}
	if esClient, eerr := esearch.New(esAddr); eerr == nil {
		esExp := NewElasticsearchExporter(esClient, serviceName)
		// Batched (not Simple) so high-throughput pipeline
		// runs don't hammer ES one bulk-index call per span.
		// SDK default batch is 512 spans / 5s tick — fine.
		traceOpts = append(traceOpts, sdktrace.WithBatcher(esExp))
		slog.Info("telemetry: elasticsearch trace exporter enabled", "addr", esAddr)
	} else {
		slog.Warn("telemetry: elasticsearch trace exporter disabled", "err", eerr)
	}

	tp := sdktrace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	metricsPath := filepath.Join(filepath.Dir(tracesFilePath()), "metrics.jsonl")
	mf, err := os.OpenFile(metricsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("telemetry: open metrics file: %w", err)
	}
	metricExp, err := stdoutmetric.New(stdoutmetric.WithWriter(mf))
	if err != nil {
		_ = mf.Close()
		return nil, fmt.Errorf("telemetry: metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExp)),
	)
	otel.SetMeterProvider(mp)

	shutdown := func(ctx context.Context) error {
		_ = tp.Shutdown(ctx)
		_ = mp.Shutdown(ctx)
		if fileCloser != nil {
			_ = fileCloser.Close()
		}
		_ = mf.Close()
		return nil
	}
	return shutdown, nil
}

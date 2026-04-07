package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/8op-org/gl1tch/internal/collector"
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
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("dev"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	// Create the feed channel.
	feedCh = make(chan FeedSpanEvent, 256)
	feedExp := NewFeedExporter(feedCh)

	var traceOpts []sdktrace.TracerProviderOption
	traceOpts = append(traceOpts, sdktrace.WithResource(res))
	traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.AlwaysSample()))

	// Wire the feed exporter as a simple span processor (immediate delivery).
	traceOpts = append(traceOpts, sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(feedExp)))

	var fileCloser io.Closer
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		otlpExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
		if err != nil {
			return nil, fmt.Errorf("telemetry: otlp trace exporter: %w", err)
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(otlpExp))
	} else {
		tracesPath := tracesFilePath()
		fileExp, closer, err := newFileExporter(tracesPath)
		if err != nil {
			// Non-fatal — fall back to discarding traces.
			_ = err
		} else {
			fileCloser = closer
			traceOpts = append(traceOpts, sdktrace.WithBatcher(fileExp))
		}
	}

	// Elasticsearch exporter — always wired in addition to whichever
	// of OTLP/file is configured above. We want EVERY span to land
	// in glitch-traces so the brain popover, Kibana Discover, and
	// any future "what just broke" query has the same source of
	// truth as the file exporter (the file is the local backstop;
	// ES is the queryable history).
	//
	// Best-effort: if the ES address from observer.yaml can't be
	// resolved or the client construction fails, we log and skip
	// the ES exporter. Spans still go to the file (or OTLP) and
	// the rest of the telemetry pipeline keeps working — losing
	// queryable history is a degradation, not a fatal error.
	if cfg, cerr := collector.LoadConfig(); cerr == nil {
		addr := cfg.Elasticsearch.Address
		if addr == "" {
			addr = "http://localhost:9200"
		}
		if esClient, eerr := esearch.New(addr); eerr == nil {
			esExp := NewElasticsearchExporter(esClient, serviceName)
			// Batched (not Simple) so high-throughput pipeline
			// runs don't hammer ES one bulk-index call per span.
			// SDK default batch is 512 spans / 5s tick — fine.
			traceOpts = append(traceOpts, sdktrace.WithBatcher(esExp))
			slog.Info("telemetry: elasticsearch trace exporter enabled", "addr", addr)
		} else {
			slog.Warn("telemetry: elasticsearch trace exporter disabled", "err", eerr)
		}
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

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// FeedSpanEvent is a minimal span summary sent to the TUI feed.
type FeedSpanEvent struct {
	RunID      string
	StepID     string
	SpanName   string
	DurationMS int64
	StatusOK   bool
}

// FeedExporter is a SpanExporter that sends span summaries to a channel
// for the TUI feed to consume. It does NOT import bubbletea.
type FeedExporter struct {
	ch chan<- FeedSpanEvent
}

// NewFeedExporter creates a FeedExporter writing to ch.
func NewFeedExporter(ch chan<- FeedSpanEvent) *FeedExporter {
	return &FeedExporter{ch: ch}
}

// ExportSpans implements sdktrace.SpanExporter.
func (f *FeedExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, s := range spans {
		runID, stepID := extractIDs(s.Attributes())
		if runID == "" && stepID == "" {
			continue
		}
		evt := FeedSpanEvent{
			RunID:      runID,
			StepID:     stepID,
			SpanName:   s.Name(),
			DurationMS: s.EndTime().Sub(s.StartTime()).Milliseconds(),
			StatusOK:   s.Status().Code != codes.Error,
		}
		select {
		case f.ch <- evt:
		default:
			// channel full — drop
		}
	}
	return nil
}

// Shutdown implements sdktrace.SpanExporter.
func (f *FeedExporter) Shutdown(_ context.Context) error { return nil }

func extractIDs(attrs []attribute.KeyValue) (runID, stepID string) {
	for _, a := range attrs {
		switch string(a.Key) {
		case "run.id":
			runID = a.Value.AsString()
		case "step.id":
			stepID = a.Value.AsString()
		}
	}
	return
}

package telemetry

import (
	"context"
	"strings"

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
	// Kind distinguishes "pipeline" spans (identified by run/step IDs) from
	// "game" spans (identified by span name prefix "game.").
	Kind string
	// GameICEClass is set for "game.evaluate" spans where ICE was triggered.
	GameICEClass string
	// GameAchievementsCount is the number of achievements unlocked in this run.
	GameAchievementsCount int
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
		// Game spans are routed by name prefix, not by run/step IDs.
		if strings.HasPrefix(s.Name(), "game.") {
			evt := FeedSpanEvent{
				SpanName:   s.Name(),
				DurationMS: s.EndTime().Sub(s.StartTime()).Milliseconds(),
				StatusOK:   s.Status().Code != codes.Error,
				Kind:       "game",
			}
			for _, a := range s.Attributes() {
				switch string(a.Key) {
				case "game.ice_class":
					evt.GameICEClass = a.Value.AsString()
				case "game.achievements_count":
					evt.GameAchievementsCount = int(a.Value.AsInt64())
				}
			}
			select {
			case f.ch <- evt:
			default:
				// channel full — drop
			}
			continue
		}

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
			Kind:       "pipeline",
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

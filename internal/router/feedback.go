package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FeedbackRecord is one routing outcome appended to the feedback log.
// The log is a JSONL file: one record per line, newest at the bottom.
type FeedbackRecord struct {
	Timestamp  string  `json:"ts"`
	Prompt     string  `json:"prompt"`
	Pipeline   string  `json:"pipeline"`             // "" = no match
	Confidence float64 `json:"confidence"`
	Method     string  `json:"method"`
	NearMiss   string  `json:"near_miss,omitempty"` // name of near-miss candidate, if any
}

// FeedbackLogger appends routing outcomes to a JSONL file.
// All methods are no-ops when the logger is disabled (path == "").
// Errors are silently discarded — feedback logging must never affect routing.
type FeedbackLogger struct {
	mu   sync.Mutex
	path string
}

// NewFeedbackLogger returns a FeedbackLogger that writes to
// feedbackDir/routing-feedback.jsonl. Pass "" to disable.
func NewFeedbackLogger(feedbackDir string) *FeedbackLogger {
	if feedbackDir == "" {
		return &FeedbackLogger{}
	}
	return &FeedbackLogger{path: filepath.Join(feedbackDir, "routing-feedback.jsonl")}
}

// Record appends a FeedbackRecord derived from result to the log.
// Safe for concurrent use. Silently swallows all errors.
func (l *FeedbackLogger) Record(prompt string, result *RouteResult) {
	if l == nil || l.path == "" || result == nil {
		return
	}

	rec := FeedbackRecord{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Prompt:     prompt,
		Method:     result.Method,
		Confidence: result.Confidence,
	}
	if result.Pipeline != nil {
		rec.Pipeline = result.Pipeline.Name
	}
	if result.NearMiss != nil {
		rec.NearMiss = result.NearMiss.Name
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

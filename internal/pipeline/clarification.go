package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/adam-stokes/orcai/internal/busd"
	"github.com/adam-stokes/orcai/internal/busd/topics"
	store "github.com/adam-stokes/orcai/internal/store"
)

// clarificationTimeout is the maximum time AskClarification will block before
// returning an error. Long enough for a user to notice and respond but short
// enough that a forgotten run does not hang indefinitely.
const clarificationTimeout = 10 * time.Minute

// AskClarification persists a ClarificationRequest to the DB, publishes a
// best-effort live notification on busd, then polls the DB every 2 seconds
// until the TUI writes an answer or the context (or clarificationTimeout) expires.
//
// runID must be the string representation of the store run ID so the TUI can
// correlate the reply with the correct panel. stepID identifies the pipeline
// step that raised the question (used when resuming after a TUI restart).
// output is the partial pipeline output accumulated up to the point the
// question was detected.
func AskClarification(ctx context.Context, runID, stepID, question, output string) (string, error) {
	st, err := store.Open()
	if err != nil {
		return "", fmt.Errorf("clarification: open store: %w", err)
	}
	defer st.Close()

	// Persist the question so it survives TUI restarts.
	if err := st.SaveClarification(runID, stepID, question, output); err != nil {
		return "", fmt.Errorf("clarification: save clarification: %w", err)
	}

	// Publish a live notification so the TUI can surface it immediately.
	// This is best-effort — if busd is down we proceed with DB polling only.
	req := store.ClarificationRequest{
		RunID:    runID,
		StepID:   stepID,
		Question: question,
		AskedAt:  time.Now(),
		Output:   output,
	}
	if sockPath, sockErr := busd.SocketPath(); sockErr == nil {
		_ = busd.PublishEvent(sockPath, topics.ClarificationRequested, req)
	}

	// Poll the DB every 2 seconds for the answer.
	deadline := time.Now().Add(clarificationTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = st.DeleteClarification(runID)
			return "", ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				_ = st.DeleteClarification(runID)
				return "", fmt.Errorf("clarification: timed out waiting for user reply")
			}
			answer, found, pollErr := st.PollClarificationAnswer(runID)
			if pollErr != nil {
				return "", fmt.Errorf("clarification: poll error: %w", pollErr)
			}
			if found {
				_ = st.DeleteClarification(runID)
				return answer, nil
			}
		}
	}
}

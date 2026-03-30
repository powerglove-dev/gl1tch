package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/adam-stokes/orcai/internal/busd"
	"github.com/adam-stokes/orcai/internal/busd/topics"
	"github.com/adam-stokes/orcai/internal/store"
)

// clarificationTimeout is the maximum time AskClarification will block before
// returning an error. Long enough for a user to notice and respond but short
// enough that a forgotten run does not hang indefinitely.
const clarificationTimeout = 10 * time.Minute

// AskClarification publishes a ClarificationRequest event on the bus, then
// blocks until a matching ClarificationReply arrives or the context (or the
// built-in clarificationTimeout) expires. The caller injects the returned
// answer string into the conversation context and resumes execution.
//
// runID must be the string representation of the store run ID so the TUI can
// correlate the reply with the correct panel.
func AskClarification(ctx context.Context, runID, question string) (string, error) {
	sockPath, err := busd.SocketPath()
	if err != nil {
		return "", fmt.Errorf("clarification: resolve socket path: %w", err)
	}

	// Publish the request so the TUI can surface it.
	req := store.ClarificationRequest{
		RunID:    runID,
		Question: question,
		AskedAt:  time.Now(),
	}
	if err := busd.PublishEvent(sockPath, topics.ClarificationRequested, req); err != nil {
		return "", fmt.Errorf("clarification: publish request: %w", err)
	}

	// Open a second connection to subscribe for the reply.
	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		return "", fmt.Errorf("clarification: subscribe for reply: %w", err)
	}
	defer conn.Close()

	reg, _ := json.Marshal(map[string]any{
		"name":      fmt.Sprintf("clarification-waiter-%s", runID),
		"subscribe": []string{topics.ClarificationReply},
	})
	if _, err := conn.Write(append(reg, '\n')); err != nil {
		return "", fmt.Errorf("clarification: register subscription: %w", err)
	}

	// Apply a hard deadline so the connection is not open indefinitely.
	deadline := time.Now().Add(clarificationTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	conn.SetReadDeadline(deadline) //nolint:errcheck

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		var frame struct {
			Event   string          `json:"event"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}
		if frame.Event != topics.ClarificationReply {
			continue
		}
		var reply store.ClarificationReply
		if err := json.Unmarshal(frame.Payload, &reply); err != nil {
			continue
		}
		if reply.RunID != runID {
			continue
		}
		return reply.Answer, nil
	}

	if err := scanner.Err(); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return "", fmt.Errorf("clarification: timed out waiting for user reply")
		}
		return "", fmt.Errorf("clarification: read error: %w", err)
	}
	return "", fmt.Errorf("clarification: connection closed before reply arrived")
}

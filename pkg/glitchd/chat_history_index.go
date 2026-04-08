// chat_history_index.go keeps the glitch-chat-history
// Elasticsearch index in sync with the SQLite workspace_messages
// table. Every time pkg/glitchd.SaveMessage is called, the
// persisted row is also indexed into ES so the observer's RAG
// path can find chat history when the user asks follow-up
// questions like "polish the draft for #1265".
//
// Design rules:
//
//   - **Fire-and-forget.** IndexChatMessage runs in a background
//     goroutine fired from SaveMessage so the chat latency is
//     untouched. Indexing failures are logged at warn but never
//     surface back to the caller — the SQLite row is the source
//     of truth, ES is an advisory search layer.
//
//   - **Deterministic _id.** The message id is used directly as
//     the ES _id (via BulkIndex's action-line handling, which
//     falls through to auto-id when the source's type isn't one
//     of the glitch-events rules — but we bypass BulkIndex
//     entirely here and call the low-level index client to set
//     _id explicitly). Re-saves of the same message (common
//     during stream-then-finalize) overwrite in place, matching
//     the "idempotent writes" invariant we enforce on glitch-events.
//
//   - **Text extraction is naive on purpose.** We walk the blocks
//     JSON, concatenate every "content" field from text blocks,
//     and ignore everything else. That keeps the ES text field
//     aligned with what the user actually *sees* in the chat pane
//     so RAG search matches human intent. Code, done-meta, and
//     other block types contribute nothing.
//
//   - **Injected message metadata is best-effort.** When the blocks
//     body happens to contain a fenced `> 📬 new attention event`
//     header we pattern-match a github URL and stamp it onto
//     event_url. The full structured metadata (event_key, source,
//     repo) lives in the frontend's Message.injected field but is
//     NOT persisted to SQLite today — once it is, this file is the
//     consumer that turns those fields into ES mapping fields.
package glitchd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
)

// IndexChatMessage upserts one workspace chat turn into
// glitch-chat-history. Called fire-and-forget from SaveMessage —
// the caller does not wait for this function and does not receive
// errors.
//
// The blocksJSON argument is the serialized Message.blocks array
// the frontend stores. We parse it, pull text content out of
// every text-type block, and index the concatenation. Other block
// types (code, done, brain, activity) are skipped on purpose
// because they either duplicate the text content or carry
// meta-information that the user doesn't type queries against.
func IndexChatMessage(ctx context.Context, id, workspaceID, role, blocksJSON string, timestampMs int64) {
	if id == "" || workspaceID == "" {
		return
	}

	text := extractTextFromBlocks(blocksJSON)
	if strings.TrimSpace(text) == "" {
		// Nothing searchable in this message — skip the index
		// write entirely. An empty chat_history row would just
		// be noise for the observer's RAG scorer.
		return
	}

	// Pattern-match a github URL if the message body contains
	// one. For injected attention messages this captures the PR
	// link inside the blockquote header (see
	// buildAttentionChatBody in glitch-desktop/app.go); for
	// user-typed messages referencing a PR link it captures
	// that too. Null-safe — findString returns "" on no match.
	eventURL := firstGitHubURL(text)

	doc := map[string]any{
		"message_id":   id,
		"workspace_id": workspaceID,
		"role":         role,
		"text":         text,
		"timestamp":    time.UnixMilli(timestampMs).UTC().Format(time.RFC3339),
	}
	if eventURL != "" {
		doc["event_url"] = eventURL
	}

	// Use the raw ES HTTP API instead of BulkIndex so we can
	// set a deterministic _id. BulkIndex's id scheme only covers
	// glitch-events types; adding a chat-history case there
	// would couple the bulk router to yet another type. Cleaner
	// to do a direct PUT here since we're writing one doc at a
	// time anyway.
	cfg, _ := capability.LoadConfig()
	addr := "http://localhost:9200"
	if cfg != nil && cfg.Elasticsearch.Address != "" {
		addr = cfg.Elasticsearch.Address
	}

	body, err := json.Marshal(doc)
	if err != nil {
		slog.Warn("chat_history_index: marshal failed",
			"id", id, "err", err)
		return
	}

	url := fmt.Sprintf("%s/%s/_doc/%s", addr, esearch.IndexChatHistory, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("chat_history_index: build request failed",
			"id", id, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 5 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		slog.Warn("chat_history_index: request failed",
			"id", id, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		slog.Warn("chat_history_index: non-2xx",
			"id", id, "status", resp.StatusCode)
	}
}

// extractTextFromBlocks walks the blocks JSON (the serialized form
// of Message.blocks) and returns the concatenation of every text
// block's content. Non-text blocks contribute nothing so the
// indexed text matches what the user reads.
//
// Tolerant of malformed input: a JSON parse error returns the
// empty string and the caller skips the index write.
func extractTextFromBlocks(blocksJSON string) string {
	blocksJSON = strings.TrimSpace(blocksJSON)
	if blocksJSON == "" {
		return ""
	}
	var blocks []struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(blocksJSON), &blocks); err != nil {
		return ""
	}
	var b strings.Builder
	for i, block := range blocks {
		if block.Type != "text" || block.Content == "" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(block.Content)
	}
	return b.String()
}

// githubURLRe matches github.com PR/issue/commit URLs. Captured
// into a package-level variable so compilation is amortized
// across calls — IndexChatMessage is fired on every workspace
// message, so a per-call Compile would be wasteful.
var githubURLRe = regexp.MustCompile(`https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/(pull|issues|commit)/[A-Za-z0-9]+`)

// firstGitHubURL returns the first github URL found in s, or ""
// when none matches. Used to stamp event_url on injected
// attention messages so the observer can filter chat history by
// the event they reference.
func firstGitHubURL(s string) string {
	return githubURLRe.FindString(s)
}

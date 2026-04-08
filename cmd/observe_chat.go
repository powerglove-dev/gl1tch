// observe_chat.go implements the chat-history maintenance
// subcommands of `glitch observe`. Two operations:
//
//	glitch observe backfill-chat
//	    Walks every row in workspace_messages (SQLite) and
//	    indexes it into glitch-chat-history (ES). Idempotent —
//	    re-runs upsert by message_id so the index stays in
//	    sync without duplicating rows.
//
//	glitch observe prune-phantoms
//	    Deletes "phantom" injection messages — the chat rows
//	    produced when the classifier briefly mis-flagged
//	    container github.pr / github.issue events as high
//	    attention before the 15:00 binary dropped them from
//	    ClassifierRelevant. Identifies phantoms by the
//	    blockquote-header pattern they all share, prunes from
//	    both SQLite and the chat-history index.
//
// Both live under `glitch observe` because that's the verb
// people associate with collected-data maintenance, alongside
// the existing `observe reset` command.
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/pkg/glitchd"
)

var (
	pruneYes    bool
	pruneDryRun bool
)

func init() {
	observeCmd.AddCommand(observeBackfillChatCmd)
	observeCmd.AddCommand(observePrunePhantomsCmd)
	observePrunePhantomsCmd.Flags().BoolVarP(&pruneYes, "yes", "y", false,
		"skip the confirmation prompt")
	observePrunePhantomsCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false,
		"show what would be pruned without deleting")
}

// ── backfill-chat ──────────────────────────────────────────────────

var observeBackfillChatCmd = &cobra.Command{
	Use:   "backfill-chat",
	Short: "Re-index every workspace_messages row into glitch-chat-history",
	Long: `Walks the SQLite workspace_messages table and indexes every row
into the glitch-chat-history Elasticsearch index. Used to backfill
chat history that was saved before the IndexChatMessage hook landed,
so the observer's RAG path can find historical assistant messages
when answering follow-up questions.

Idempotent: re-runs upsert by message_id, so it's safe to run any
time you suspect the index has drifted from the SQLite source of
truth (e.g. after restoring from a backup or after the index was
recreated by EnsureIndices).`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()

		st, err := store.Open()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer st.Close()

		msgs, err := st.ListAllWorkspaceMessages(ctx)
		if err != nil {
			return fmt.Errorf("list messages: %w", err)
		}

		if len(msgs) == 0 {
			fmt.Fprintln(os.Stderr, "no messages to backfill")
			return nil
		}

		fmt.Fprintf(os.Stderr, "backfilling %d messages into glitch-chat-history…\n", len(msgs))
		indexed := 0
		for _, m := range msgs {
			// IndexChatMessage is fire-and-forget by design (it
			// gets called from SaveMessage in a goroutine), but
			// here we want synchronous behavior so the user sees
			// a final count. Calling it directly on the current
			// goroutine is the contract — the function itself
			// performs a blocking HTTP PUT and returns on its
			// own. Any failures are logged at warn but don't
			// abort the backfill.
			glitchd.IndexChatMessage(ctx, m.ID, m.WorkspaceID, m.Role, m.BlocksJSON, m.Timestamp)
			indexed++
		}
		fmt.Fprintf(os.Stderr, "done — indexed %d/%d messages\n", indexed, len(msgs))
		return nil
	},
}

// ── prune-phantoms ────────────────────────────────────────────────

var observePrunePhantomsCmd = &cobra.Command{
	Use:   "prune-phantoms",
	Short: "Delete stale github.pr / github.issue container-event chat injections",
	Long: `Removes the "phantom" assistant chat messages produced when the
attention classifier briefly mis-flagged github.pr and github.issue
container events as high-attention. Those events were dropped from
ClassifierRelevant in the 15:00 binary, but the historical chat
messages they generated still sit in workspace_messages and
glitch-chat-history.

A phantom is detected by its blockquote header shape: an injection
where the title slot starts with "#" (the bare PR/issue title) AND
the message blocks reference no github.pr_review / github.pr_comment
event_key. Real injections start with "Review on PR" or
"Comment on" or "⚙️ deep analysis is off".

Use --dry-run to preview which messages would be removed.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
		defer cancel()

		st, err := store.Open()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer st.Close()

		msgs, err := st.ListAllWorkspaceMessages(ctx)
		if err != nil {
			return fmt.Errorf("list messages: %w", err)
		}

		var phantoms []store.WorkspaceMessage
		for _, m := range msgs {
			if isPhantomInjection(m.Role, m.BlocksJSON) {
				phantoms = append(phantoms, m)
			}
		}

		if len(phantoms) == 0 {
			fmt.Fprintln(os.Stderr, "no phantom injections found")
			return nil
		}

		fmt.Fprintf(os.Stderr, "found %d phantom injection(s):\n\n", len(phantoms))
		for _, m := range phantoms {
			ts := time.UnixMilli(m.Timestamp).UTC().Format(time.RFC3339)
			preview := previewBlockText(m.BlocksJSON, 80)
			fmt.Fprintf(os.Stderr, "  %s · %s · %s\n", ts, m.ID, preview)
		}
		fmt.Fprintln(os.Stderr)

		if pruneDryRun {
			fmt.Fprintln(os.Stderr, "(dry-run — nothing deleted)")
			return nil
		}
		if !pruneYes {
			fmt.Fprint(os.Stderr, "Delete these from SQLite AND glitch-chat-history? [y/N] ")
			var answer string
			_, _ = fmt.Fscanln(os.Stdin, &answer)
			if answer != "y" && answer != "Y" && answer != "yes" {
				return fmt.Errorf("aborted")
			}
		}

		// Delete from SQLite first; if SQLite fails we leave ES
		// alone. If SQLite succeeds we always try ES too — an
		// orphaned ES doc is less bad than a missing SQLite row.
		sqliteDeleted, esDeleted := 0, 0
		for _, m := range phantoms {
			if err := st.DeleteWorkspaceMessage(ctx, m.ID); err != nil {
				fmt.Fprintf(os.Stderr, "warn: sqlite delete %s: %v\n", m.ID, err)
				continue
			}
			sqliteDeleted++
			if err := deleteChatHistoryDoc(ctx, m.ID); err != nil {
				fmt.Fprintf(os.Stderr, "warn: es delete %s: %v\n", m.ID, err)
				continue
			}
			esDeleted++
		}
		fmt.Fprintf(os.Stderr, "pruned %d/%d sqlite, %d/%d es\n",
			sqliteDeleted, len(phantoms), esDeleted, len(phantoms))
		return nil
	},
}

// isPhantomInjection returns true when a message looks like a
// stale `github.pr` / `github.issue` injection from before the
// 15:00 classifier filter landed. Detection is purely structural:
//
//   - role must be "assistant"
//   - block text must contain the attention header
//     "📬 **new attention event**"
//   - the title slot (the segment after "workspace · ") must
//     start with "#" or "[#" — the github.pr / github.issue
//     events render their title as "#1266 feat: …" (or
//     "[#1266 feat: …](url)" with the markdown link), whereas
//     real review/comment injections render as
//     "Review on PR #1266: COMMENTED" or "Comment on #1266 …"
//     which start with R or C, not #
//
// Defensive: never matches messages that contain "Review on" or
// "Comment on" anywhere, even if they also contain a "#" prefix
// somewhere. Real injections always have one of those phrases.
func isPhantomInjection(role, blocksJSON string) bool {
	if role != "assistant" {
		return false
	}
	if !strings.Contains(blocksJSON, "📬 **new attention event**") {
		return false
	}
	// Real injection sentinels — if any of these appear, it's
	// not a phantom regardless of how the title looks.
	if strings.Contains(blocksJSON, "Review on PR") ||
		strings.Contains(blocksJSON, "Review on #") ||
		strings.Contains(blocksJSON, "Comment on PR") ||
		strings.Contains(blocksJSON, "Comment on #") ||
		strings.Contains(blocksJSON, "⚙️ **deep analysis is off**") {
		return false
	}
	// Phantom sentinels — bare "#NNN" or "[#NNN" right after the
	// "workspace · " header. The bracket form is the post-14:44
	// markdown-link rendering; the bare form is older.
	if strings.Contains(blocksJSON, "workspace · #") ||
		strings.Contains(blocksJSON, "workspace · [#") {
		return true
	}
	return false
}

// previewBlockText pulls a short preview out of the first text
// block for the SQL row listing. Caps to maxLen runes so the
// listing stays one line per row.
func previewBlockText(blocksJSON string, maxLen int) string {
	// Find the first "content":" substring and take the next
	// maxLen chars before any closing quote. Lazy parser — the
	// preview is human-eyeball-only so a structurally-perfect
	// extraction would be over-engineered.
	idx := strings.Index(blocksJSON, `"content":"`)
	if idx < 0 {
		return "(no text block)"
	}
	rest := blocksJSON[idx+len(`"content":"`):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		end = len(rest)
	}
	preview := rest[:end]
	if len(preview) > maxLen {
		preview = preview[:maxLen] + "…"
	}
	// Remove the leading "> 📬 ..." blockquote prefix so the
	// preview shows the meaningful title slot.
	preview = strings.ReplaceAll(preview, `\n`, " ")
	return preview
}

// deleteChatHistoryDoc removes one document by id from the
// glitch-chat-history index. Uses the raw esearch client because
// the package's BulkIndex has no per-id delete path. A 404 is
// not an error — older messages might never have been indexed
// (the IndexChatMessage hook only fires on save events from the
// new binary), so the prune command must tolerate missing rows.
func deleteChatHistoryDoc(ctx context.Context, messageID string) error {
	addr := "http://localhost:9200"
	if cfg, _ := capability.LoadConfig(); cfg != nil && cfg.Elasticsearch.Address != "" {
		addr = cfg.Elasticsearch.Address
	}
	es, err := esearch.New(addr)
	if err != nil {
		return err
	}
	// DeleteByQuery on a single _id keeps us off the document API
	// surface (which esearch.Client doesn't expose) and lets us
	// reuse the existing query path. Match by message_id keyword.
	if _, err := es.DeleteByQuery(ctx,
		[]string{esearch.IndexChatHistory},
		map[string]any{"term": map[string]any{"message_id": messageID}},
	); err != nil {
		return err
	}
	return nil
}

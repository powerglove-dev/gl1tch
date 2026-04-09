// threads.go is the CLI surface for the chat-threads + research-loop
// stack the desktop drives via Wails. It exists so:
//
//   - The exact same code paths the desktop hits are exercisable from
//     the terminal, end-to-end, without launching the Wails app. This
//     is the smoke test rig the user can run by hand and CI can run on
//     a schedule to detect regressions in the threaded research path.
//
//   - `glitch ask` and `glitch threads` together cover everything the
//     desktop chat can do — pose a question, get a grounded answer,
//     spawn a thread on a previous answer, ask a follow-up scoped to
//     that thread, list / show threads, all with the same provider/
//     model/cwd plumbing the desktop uses.
//
// The thread store is in-memory in v1 (`pkg/glitchd/threads.go` keeps
// one ThreadHosts registry per process), so threads created by `glitch
// threads new` only persist for the lifetime of the parent shell
// process. That's the right tradeoff for v1 smoke testing — no SQLite
// schema churn, no leftover state — and matches the desktop's current
// behaviour. Persistence lands in a follow-up.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

// threadsCmd is the parent for the thread sub-commands. It is named
// `threads` (plural) to mirror the desktop's "threads side pane" copy
// and to leave room for a future `glitch thread <id>` shorthand.
var threadsCmd = &cobra.Command{
	Use:   "threads",
	Short: "Drive the threaded research chat from the CLI",
	Long: `Drive the threaded research chat — the same one glitch-desktop's
chat panel uses — from the terminal. Every sub-command operates on a
single workspace's in-memory ThreadHost (constructed lazily on first
access) so the data the desktop sees and the data the CLI sees come
from the same store.

Smoke-test recipe (verifies the full plan→gather→draft→critique→score
loop end-to-end against a workspace's actual repo):

  glitch threads new -w <workspace-id> "is this PR updated yet?"
  # → emits {thread_id, parent_id, draft, evidence_count, composite}
  glitch threads ask -w <workspace-id> -t <thread-id> "what changed?"
  glitch threads show -w <workspace-id> -t <thread-id>

Scripted self-improvement: pipe the JSON output of 'show' into jq /
'glitch attention' to teach the brain stats engine which threads
landed and which didn't.`,
}

var (
	threadsWorkspaceID string
	threadsThreadID    string
	threadsJSONOut     bool
)

// threadsHosts is a process-singleton ThreadHosts registry the CLI
// commands share. The first sub-command call constructs it; subsequent
// calls within the same `glitch` invocation reuse the same map so
// `glitch threads ask` can find a thread spawned by `glitch threads new`.
//
// Note: in v1 the registry does not survive across `glitch` invocations
// because the ThreadStore is in-memory. Smoke tests must chain
// sub-commands inside a single shell wrapper or rely on the IDs the
// `new` sub-command prints.
var threadsHosts *glitchd.ThreadHosts

func ensureThreadsHosts() *glitchd.ThreadHosts {
	if threadsHosts == nil {
		threadsHosts = glitchd.NewThreadHosts()
	}
	return threadsHosts
}

func init() {
	rootCmd.AddCommand(threadsCmd)
	threadsCmd.PersistentFlags().StringVarP(&threadsWorkspaceID, "workspace", "w", "",
		"workspace id (defaults to GLITCH_WORKSPACE_ID env var)")
	threadsCmd.PersistentFlags().BoolVar(&threadsJSONOut, "json", false,
		"emit JSON envelopes instead of human-readable output")

	threadsCmd.AddCommand(threadsNewCmd)
	threadsCmd.AddCommand(threadsAskCmd)
	threadsCmd.AddCommand(threadsListCmd)
	threadsCmd.AddCommand(threadsShowCmd)
	threadsCmd.AddCommand(threadsSmokeCmd)
	threadsCmd.AddCommand(threadsFeedbackCmd)

	threadsAskCmd.Flags().StringVarP(&threadsThreadID, "thread", "t", "",
		"thread id (printed by `glitch threads new`)")
	threadsShowCmd.Flags().StringVarP(&threadsThreadID, "thread", "t", "",
		"thread id (printed by `glitch threads new`)")
	threadsFeedbackCmd.Flags().StringVarP(&threadsThreadID, "thread", "t", "",
		"thread id (printed by `glitch threads new`)")
	threadsFeedbackCmd.Flags().BoolVar(&threadsFeedbackAccept, "accept", false,
		"thumbs-up — explicit accept of the most recent research result in the thread")
	threadsFeedbackCmd.Flags().BoolVar(&threadsFeedbackReject, "reject", false,
		"thumbs-down — explicit reject of the most recent research result in the thread")
	threadsFeedbackCmd.Flags().StringVar(&threadsFeedbackQueryID, "query-id", "",
		"override the query id the feedback applies to (defaults to the thread's last research call)")
	threadsFeedbackCmd.Flags().StringVar(&threadsFeedbackQuestion, "question", "",
		"the question the feedback applies to (used by the brain hints reader to bias future similar questions)")
}

var (
	threadsFeedbackAccept   bool
	threadsFeedbackReject   bool
	threadsFeedbackQueryID  string
	threadsFeedbackQuestion string
)

// ── threads feedback ────────────────────────────────────────────────────────

var threadsFeedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Record an explicit accept/reject for the most recent research result in a thread",
	Long: `Writes one research_feedback event tagged to the supplied thread.
The brain hints reader weights this above the composite proxy:

  --accept boosts the picks for future similar questions
  --reject filters the picks out of future hints entirely

This is the explicit label the loop has been missing — composites
are a useful proxy but the user's actual judgment is the ground
truth. Pipe accept/reject calls into a CI cron and the brain learns
from real outcomes.

Smoke recipe (the canonical "teach me you learned this" sequence):

  glitch threads new -w <ws> "what prs are open"
  # → emits thread_id and the loop picks github-prs
  glitch threads feedback -w <ws> -t <thread-id> --accept \\
    --question "what prs are open"
  # next time you ask anything pr-shaped, the planner will see
  # 👍 picks=[github-prs] in its hint block and bias toward it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspaceID()
		if err != nil {
			return err
		}
		if threadsThreadID == "" {
			return fmt.Errorf("thread id is required (-t)")
		}
		if threadsFeedbackAccept == threadsFeedbackReject {
			return fmt.Errorf("exactly one of --accept or --reject is required")
		}
		envelope := ensureThreadsHosts().RecordResearchFeedback(
			ws, threadsThreadID, threadsFeedbackQueryID, threadsFeedbackQuestion, threadsFeedbackAccept,
		)
		if threadsJSONOut {
			fmt.Println(envelope)
			return nil
		}
		return prettyPrintEnvelope(envelope, "feedback")
	},
}

// resolveWorkspaceID resolves the workspace id the user wants the
// command to operate against. Falls back to GLITCH_WORKSPACE_ID env var
// so smoke tests can `export GLITCH_WORKSPACE_ID=...` once and chain
// sub-commands without re-passing -w.
func resolveWorkspaceID() (string, error) {
	if threadsWorkspaceID != "" {
		return threadsWorkspaceID, nil
	}
	if v := os.Getenv("GLITCH_WORKSPACE_ID"); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("workspace id is required (-w or GLITCH_WORKSPACE_ID)")
}

// ── threads new ─────────────────────────────────────────────────────────────

var threadsNewCmd = &cobra.Command{
	Use:   "new <question>",
	Short: "Pose a question and get back a grounded answer + auto-spawned thread",
	Long: `Runs the research loop end-to-end the same way the desktop does
when the user types a freeform line in the chat. Emits the parent
message id and the auto-spawned thread id so subsequent CLI calls
(or jq pipelines) can drill into the result.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspaceID()
		if err != nil {
			return err
		}
		question := strings.Join(args, " ")
		envelope := ensureThreadsHosts().DispatchSlash(ws, question, "main")
		if threadsJSONOut {
			fmt.Println(envelope)
			return nil
		}
		return prettyPrintEnvelope(envelope, "research")
	},
}

// ── threads ask ─────────────────────────────────────────────────────────────

var threadsAskCmd = &cobra.Command{
	Use:   "ask <followup>",
	Short: "Ask a follow-up question scoped to an existing thread",
	Long: `Runs the research loop again with the supplied question, but
appends the result to an existing thread instead of spawning a new
top-level summary. Mirrors the desktop's "type in the side pane"
flow exactly.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspaceID()
		if err != nil {
			return err
		}
		if threadsThreadID == "" {
			return fmt.Errorf("thread id is required (-t)")
		}
		question := strings.Join(args, " ")
		envelope := ensureThreadsHosts().DispatchSlash(ws, question, "thread:"+threadsThreadID)
		if threadsJSONOut {
			fmt.Println(envelope)
			return nil
		}
		return prettyPrintEnvelope(envelope, "follow-up")
	},
}

// ── threads list ────────────────────────────────────────────────────────────

var threadsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every thread the workspace knows about",
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspaceID()
		if err != nil {
			return err
		}
		raw := ensureThreadsHosts().ListThreads(ws)
		if threadsJSONOut {
			fmt.Println(raw)
			return nil
		}
		var threads []map[string]any
		if err := json.Unmarshal([]byte(raw), &threads); err != nil {
			return fmt.Errorf("parse threads: %w", err)
		}
		if len(threads) == 0 {
			fmt.Println("(no threads in this workspace yet — try `glitch threads new`)")
			return nil
		}
		for _, t := range threads {
			fmt.Printf("%s  parent=%s  state=%s  last=%s\n",
				str(t["id"]), str(t["parent_message_id"]), str(t["state"]), str(t["last_activity_at"]))
		}
		return nil
	},
}

// ── threads show ────────────────────────────────────────────────────────────

var threadsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the messages inside a single thread",
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspaceID()
		if err != nil {
			return err
		}
		if threadsThreadID == "" {
			return fmt.Errorf("thread id is required (-t)")
		}
		raw := ensureThreadsHosts().ThreadMessages(ws, threadsThreadID)
		if threadsJSONOut {
			fmt.Println(raw)
			return nil
		}
		var msgs []map[string]any
		if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
			return fmt.Errorf("parse messages: %w", err)
		}
		for _, m := range msgs {
			fmt.Printf("── %s ──\n", str(m["role"]))
			payload, _ := m["payload"].(map[string]any)
			if body, ok := payload["body"].(string); ok && body != "" {
				fmt.Println(body)
				continue
			}
			if title, ok := payload["title"].(string); ok && title != "" {
				fmt.Println(title)
				continue
			}
			b, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(b))
		}
		return nil
	},
}

// ── threads smoke ───────────────────────────────────────────────────────────

var threadsSmokeCmd = &cobra.Command{
	Use:   "smoke",
	Short: "End-to-end smoke test of the threaded research path",
	Long: `Runs the canonical smoke sequence — new → ask → list → show — in
one shot, exercising the full plan/gather/draft/critique/score loop
plus the chatui ThreadStore plus the per-workspace cwd injection.

This is the rig CI runs against a workspace that points at a real git
repo. Exit code is 0 only when every step succeeds AND the resulting
thread contains at least one assistant message that mentions a
researcher source name (proves grounding ran end-to-end).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspaceID()
		if err != nil {
			return err
		}
		hosts := ensureThreadsHosts()

		// Step 0: ground-truth lookup. Capture the workspace's primary
		// directory and the most recent commits there. The smoke
		// assertion compares those SHAs against what the loop returns,
		// so a passing test proves the loop ran git-log inside the
		// workspace's primary repo — not the desktop binary's own cwd.
		// This is the assertion the previous "any researcher source
		// name appears in output" check missed.
		st, err := glitchd.OpenStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		w, err := st.GetWorkspace(context.Background(), ws)
		if err != nil {
			return fmt.Errorf("get workspace: %w", err)
		}
		if w.PrimaryDirectory == "" {
			return fmt.Errorf("smoke: workspace %s has no primary directory", ws)
		}
		fmt.Fprintf(os.Stderr, "[0/4] primary=%s\n", w.PrimaryDirectory)

		groundTruthSHAs, err := recentSHAs(w.PrimaryDirectory, 10)
		if err != nil {
			return fmt.Errorf("smoke: read primary repo log: %w", err)
		}
		if len(groundTruthSHAs) == 0 {
			return fmt.Errorf("smoke: primary repo %s has no commits", w.PrimaryDirectory)
		}
		fmt.Fprintf(os.Stderr, "      ground-truth head=%s\n", groundTruthSHAs[0])

		// Step 1: spawn a top-level thread with a researchable question.
		fmt.Fprintln(os.Stderr, "[1/4] threads new …")
		newEnv := hosts.DispatchSlash(ws, "what are the most recent commits?", "main")
		var newDecoded map[string]any
		if err := json.Unmarshal([]byte(newEnv), &newDecoded); err != nil {
			return fmt.Errorf("decode new envelope: %w (raw=%s)", err, newEnv)
		}
		threadID, _ := newDecoded["thread_id"].(string)
		if threadID == "" {
			return fmt.Errorf("smoke: new envelope missing thread_id: %s", newEnv)
		}
		fmt.Fprintf(os.Stderr, "      thread_id=%s\n", threadID)

		// Step 2: follow-up question scoped to the new thread.
		fmt.Fprintln(os.Stderr, "[2/4] threads ask …")
		askEnv := hosts.DispatchSlash(ws, "who authored them?", "thread:"+threadID)
		if !strings.Contains(askEnv, `"ok":true`) {
			return fmt.Errorf("smoke: ask failed: %s", askEnv)
		}

		// Step 3: list threads must include the one we just spawned.
		fmt.Fprintln(os.Stderr, "[3/4] threads list …")
		listRaw := hosts.ListThreads(ws)
		if !strings.Contains(listRaw, threadID) {
			return fmt.Errorf("smoke: list missing thread_id %s: %s", threadID, listRaw)
		}

		// Step 4: show thread messages, then assert (a) the thread
		// references a registered researcher source name and (b) at
		// least one of the workspace's actual ground-truth SHAs
		// appears in the gathered evidence. (b) is the assertion
		// that proves the cwd injection landed in the right repo;
		// without it, a regression that runs git-log against the
		// desktop binary's cwd would still pass step (a) silently.
		fmt.Fprintln(os.Stderr, "[4/4] threads show …")
		showRaw := hosts.ThreadMessages(ws, threadID)
		grounded := false
		for _, src := range []string{"git-log", "git-status", "github-prs", "github-issues"} {
			if strings.Contains(showRaw, src) {
				grounded = true
				break
			}
		}
		if !grounded {
			return fmt.Errorf("smoke: thread did not reference any researcher source — gather stage may have failed.\nshow=%s", showRaw)
		}
		matched := ""
		for _, sha := range groundTruthSHAs {
			if sha != "" && strings.Contains(showRaw, sha) {
				matched = sha
				break
			}
		}
		if matched == "" {
			return fmt.Errorf("smoke: gathered evidence does not contain ANY of the workspace's recent SHAs — the loop ran git-log against the wrong directory.\nprimary=%s\nground-truth=%v\nshow=%s",
				w.PrimaryDirectory, groundTruthSHAs, showRaw)
		}
		fmt.Fprintf(os.Stderr, "✓ smoke: threaded research path is healthy (matched %s)\n", matched)
		return nil
	},
}

// recentSHAs returns up to n recent short SHAs from the git repo at
// dir. The smoke command uses this as ground truth: the loop's
// gathered evidence MUST contain at least one of these SHAs, otherwise
// the cwd injection regressed and git-log ran in the wrong directory.
func recentSHAs(dir string, n int) ([]string, error) {
	out, err := exec.Command("git", "-C", dir, "log",
		"--pretty=format:%h", fmt.Sprintf("-n%d", n)).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	clean := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			clean = append(clean, l)
		}
	}
	return clean, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

// prettyPrintEnvelope renders a {ok,thread_id,parent_id,error} envelope
// in human-readable form. The CLI defaults to this; --json prints raw.
func prettyPrintEnvelope(envelope, label string) error {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(envelope), &decoded); err != nil {
		fmt.Println(envelope)
		return nil
	}
	if okField, ok := decoded["ok"].(bool); ok && !okField {
		return fmt.Errorf("%s failed: %v", label, decoded["error"])
	}
	if tid, ok := decoded["thread_id"].(string); ok && tid != "" {
		fmt.Printf("thread_id=%s\n", tid)
	}
	if pid, ok := decoded["parent_id"].(string); ok && pid != "" {
		fmt.Printf("parent_id=%s\n", pid)
	}
	if detail, ok := decoded["detail"].(string); ok && detail != "" {
		fmt.Printf("detail=%s\n", detail)
	}
	return nil
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

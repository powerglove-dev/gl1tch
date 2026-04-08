package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// smoke_workspaces_test.go exercises the desktop-chat flow against two
// fixture workspaces named "robots" and "ensemble" — mirroring the
// user's real workspace layout — and verifies that the Agent can
// surface collector-style data (fake PRs, fake issue comments) via
// tool calls. No real network, no real gh CLI, no real Ollama: the
// fixtures are JSON on disk, the capabilities read them, the
// ToolProvider is scripted.
//
// The goal is confidence that "I open glitch-desktop, switch to the
// robots workspace, ask about open PRs" produces the right shape of
// conversation — without any hardcoded preamble telling the model
// what glitch is.

// fakePRDoc is the shape each fixture PR takes. Deliberately slim so
// the test is readable; a real collector document has more fields.
type fakePRDoc struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author string `json:"author"`
	State  string `json:"state"`
}

type fakeCommentDoc struct {
	PR     int    `json:"pr"`
	Author string `json:"author"`
	Body   string `json:"body"`
}

// seedWorkspaceWithFakeData writes a collector-style JSON bundle into
// <root>/.glitch/collector/ so a capability can read it as if it were
// the output of the real github collector. We keep the fixture format
// dead simple — one file per collection — so the test stays legible.
func seedWorkspaceWithFakeData(t *testing.T, root string, prs []fakePRDoc, comments []fakeCommentDoc) {
	t.Helper()
	dir := filepath.Join(root, ".glitch", "collector")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	writeJSON := func(name string, v any) {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeJSON("prs.json", prs)
	writeJSON("issue_comments.json", comments)
}

// listPRsCap is an on-demand capability that reads the fake
// prs.json fixture out of a workspace's .glitch/collector directory
// and returns a short human summary as a Stream event. It is the
// smoke-test stand-in for the real github PR collector + a future
// "workspace.prs" tool.
type listPRsCap struct {
	workspace string
}

func (c *listPRsCap) Manifest() Manifest {
	return Manifest{
		Name:        "list_prs",
		Description: "List open pull requests in the current workspace. Returns one line per PR.",
		Trigger:     Trigger{Mode: TriggerOnDemand},
		Sink:        Sink{Stream: true},
	}
}

func (c *listPRsCap) Invoke(_ context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 2)
	go func() {
		defer close(ch)
		path := filepath.Join(c.workspace, ".glitch", "collector", "prs.json")
		data, err := os.ReadFile(path)
		if err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		var prs []fakePRDoc
		if err := json.Unmarshal(data, &prs); err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		lines := make([]string, 0, len(prs))
		for _, p := range prs {
			lines = append(lines, fmt.Sprintf("#%d %s (%s by @%s)", p.Number, p.Title, p.State, p.Author))
		}
		sort.Strings(lines)
		ch <- Event{Kind: EventStream, Text: strings.Join(lines, "\n")}
	}()
	return ch, nil
}

// listIssueCommentsCap reads the fake issue_comments.json fixture.
// Same shape as listPRsCap — this is intentional: it shows that adding
// a new collector-backed tool is a ~20-line exercise once the Agent +
// registry plumbing is in place.
type listIssueCommentsCap struct {
	workspace string
}

func (c *listIssueCommentsCap) Manifest() Manifest {
	return Manifest{
		Name:        "list_issue_comments",
		Description: "List recent issue comments in the current workspace. Returns one line per comment.",
		Trigger:     Trigger{Mode: TriggerOnDemand},
		Sink:        Sink{Stream: true},
	}
}

func (c *listIssueCommentsCap) Invoke(_ context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event, 2)
	go func() {
		defer close(ch)
		path := filepath.Join(c.workspace, ".glitch", "collector", "issue_comments.json")
		data, err := os.ReadFile(path)
		if err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		var comments []fakeCommentDoc
		if err := json.Unmarshal(data, &comments); err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		lines := make([]string, 0, len(comments))
		for _, cm := range comments {
			lines = append(lines, fmt.Sprintf("PR #%d @%s: %s", cm.PR, cm.Author, cm.Body))
		}
		ch <- Event{Kind: EventStream, Text: strings.Join(lines, "\n")}
	}()
	return ch, nil
}

// buildCollectorAgent is the workspace-aware equivalent of
// buildWorkspaceAgent: it registers the PR + comment capabilities
// rooted at the given workspace and returns an Agent ready to run a
// scripted conversation.
func buildCollectorAgent(t *testing.T, workspace string, replies []Reply) (*Agent, *scriptedProvider) {
	t.Helper()
	reg := NewRegistry()
	if err := reg.Register(&listPRsCap{workspace: workspace}); err != nil {
		t.Fatalf("register list_prs: %v", err)
	}
	if err := reg.Register(&listIssueCommentsCap{workspace: workspace}); err != nil {
		t.Fatalf("register list_issue_comments: %v", err)
	}
	runner := NewRunner(reg, nil)
	prov := &scriptedProvider{replies: replies}
	return NewAgent(reg, runner, prov), prov
}

// makeWorkspaceWithFakes is a tiny helper that creates a temp dir
// named like the user's real workspace ("robots" / "ensemble") and
// seeds it with the fake collector fixtures the test needs.
func makeWorkspaceWithFakes(t *testing.T, name string, prs []fakePRDoc, comments []fakeCommentDoc) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", root, err)
	}
	seedWorkspaceWithFakeData(t, root, prs, comments)
	return root
}

// TestSmoke_RobotsWorkspacePRs is the "open glitch-desktop in the
// robots workspace and ask about open PRs" scenario. The fake
// collector fixtures contain two PRs; the scripted model calls
// list_prs and answers with a summary the test can assert against.
func TestSmoke_RobotsWorkspacePRs(t *testing.T) {
	robots := makeWorkspaceWithFakes(t, "robots",
		[]fakePRDoc{
			{Number: 42, Title: "Add gripper calibration", Author: "alice", State: "open"},
			{Number: 43, Title: "Fix IMU drift", Author: "bob", State: "open"},
		},
		nil,
	)

	agent, prov := buildCollectorAgent(t, robots, []Reply{
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_prs", Arguments: map[string]any{"input": ""}}}},
		{Text: "two open PRs: #42 gripper calibration, #43 IMU drift"},
	})

	var buf strings.Builder
	out, err := agent.Send(context.Background(), "what PRs are open in robots?", &buf)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Tool result from the first turn must contain both fake PRs.
	if len(prov.calls) != 2 {
		t.Fatalf("chat calls = %d, want 2", len(prov.calls))
	}
	var toolResult string
	for _, m := range prov.calls[1].msgs {
		if m.Role == RoleTool && m.Name == "list_prs" {
			toolResult = m.Content
		}
	}
	if !strings.Contains(toolResult, "#42") || !strings.Contains(toolResult, "#43") {
		t.Errorf("list_prs tool result missing fake PRs: %q", toolResult)
	}
	// And the final assistant reply carried something the user can read.
	if !strings.Contains(out, "#42") && !strings.Contains(out, "gripper") {
		t.Errorf("final reply did not reference fake PR content: %q", out)
	}
}

// TestSmoke_EnsembleWorkspaceComments exercises the issue-comments
// tool against a different workspace, to prove per-workspace isolation
// works alongside the PR smoke test. Nothing from robots can bleed
// into this conversation — separate Agent, separate registry,
// separate fixture root.
func TestSmoke_EnsembleWorkspaceComments(t *testing.T) {
	ensemble := makeWorkspaceWithFakes(t, "ensemble",
		nil,
		[]fakeCommentDoc{
			{PR: 7, Author: "carol", Body: "ship it when CI is green"},
			{PR: 7, Author: "dave", Body: "nit: rename this var"},
		},
	)

	agent, prov := buildCollectorAgent(t, ensemble, []Reply{
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_issue_comments", Arguments: map[string]any{"input": ""}}}},
		{Text: "two recent comments on PR #7"},
	})

	if _, err := agent.Send(context.Background(), "any new comments on ensemble?", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Second-turn history must carry both fake comments verbatim so the
	// model had a chance to reason about them.
	var toolResult string
	for _, m := range prov.calls[1].msgs {
		if m.Role == RoleTool && m.Name == "list_issue_comments" {
			toolResult = m.Content
		}
	}
	if !strings.Contains(toolResult, "carol") || !strings.Contains(toolResult, "dave") {
		t.Errorf("comment tool result missing fixture authors: %q", toolResult)
	}
	if !strings.Contains(toolResult, "ship it") || !strings.Contains(toolResult, "nit: rename") {
		t.Errorf("comment tool result missing fixture bodies: %q", toolResult)
	}
}

// TestSmoke_RobotsAndEnsembleIsolation is the side-by-side version:
// both workspaces are set up with distinct fake data, each gets its
// own Agent, and we assert neither chat sees the other's fixtures.
// This is the guardrail behind the "desktop user switches workspace"
// UX — the new chat must not need a global system prompt enumerating
// every workspace the user has.
func TestSmoke_RobotsAndEnsembleIsolation(t *testing.T) {
	robots := makeWorkspaceWithFakes(t, "robots",
		[]fakePRDoc{{Number: 1, Title: "robots-only", Author: "alice", State: "open"}},
		nil,
	)
	ensemble := makeWorkspaceWithFakes(t, "ensemble",
		[]fakePRDoc{{Number: 99, Title: "ensemble-only", Author: "zed", State: "open"}},
		nil,
	)

	agentR, provR := buildCollectorAgent(t, robots, []Reply{
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_prs", Arguments: map[string]any{"input": ""}}}},
		{Text: "robots PRs listed"},
	})
	agentE, provE := buildCollectorAgent(t, ensemble, []Reply{
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_prs", Arguments: map[string]any{"input": ""}}}},
		{Text: "ensemble PRs listed"},
	})

	if _, err := agentR.Send(context.Background(), "what PRs", nil); err != nil {
		t.Fatalf("robots Send: %v", err)
	}
	if _, err := agentE.Send(context.Background(), "what PRs", nil); err != nil {
		t.Fatalf("ensemble Send: %v", err)
	}

	robotsResult := lastToolContent(provR.calls, "list_prs")
	ensembleResult := lastToolContent(provE.calls, "list_prs")

	if !strings.Contains(robotsResult, "robots-only") {
		t.Errorf("robots agent did not see its own PR: %q", robotsResult)
	}
	if strings.Contains(robotsResult, "ensemble-only") {
		t.Errorf("robots agent leaked ensemble PR: %q", robotsResult)
	}
	if !strings.Contains(ensembleResult, "ensemble-only") {
		t.Errorf("ensemble agent did not see its own PR: %q", ensembleResult)
	}
	if strings.Contains(ensembleResult, "robots-only") {
		t.Errorf("ensemble agent leaked robots PR: %q", ensembleResult)
	}
}

// TestSmoke_MultiTurnCollectorChat mimics a realistic desktop session:
// the user asks about PRs, then about issue comments, then follows
// up on a specific PR. All three turns share one Agent and one
// history — nothing reaches for a static system prompt.
func TestSmoke_MultiTurnCollectorChat(t *testing.T) {
	robots := makeWorkspaceWithFakes(t, "robots",
		[]fakePRDoc{{Number: 42, Title: "Add gripper calibration", Author: "alice", State: "open"}},
		[]fakeCommentDoc{{PR: 42, Author: "reviewer", Body: "LGTM after rebase"}},
	)

	agent, prov := buildCollectorAgent(t, robots, []Reply{
		// Turn 1: list PRs.
		{ToolCalls: []ToolCall{{ID: "c0", Name: "list_prs", Arguments: map[string]any{"input": ""}}}},
		{Text: "one open PR: #42 gripper calibration"},
		// Turn 2: list comments.
		{ToolCalls: []ToolCall{{ID: "c1", Name: "list_issue_comments", Arguments: map[string]any{"input": ""}}}},
		{Text: "one comment on #42: LGTM after rebase"},
		// Turn 3: the model has enough context to answer without a tool.
		{Text: "yes, #42 is ready — reviewer said LGTM after a rebase"},
	})

	turns := []string{
		"what PRs are open?",
		"any comments?",
		"is #42 ready to merge?",
	}
	for i, t_ := range turns {
		if _, err := agent.Send(context.Background(), t_, nil); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}

	// Three user messages must appear in history, in order.
	var userMsgs []string
	for _, m := range agent.History() {
		if m.Role == RoleUser {
			userMsgs = append(userMsgs, m.Content)
		}
	}
	if len(userMsgs) != 3 {
		t.Fatalf("user messages in history = %d, want 3: %v", len(userMsgs), userMsgs)
	}
	for i, want := range turns {
		if userMsgs[i] != want {
			t.Errorf("user msg[%d] = %q, want %q", i, userMsgs[i], want)
		}
	}

	// The final provider call should have seen both tool-result
	// messages, proving collector output accumulates across the chat.
	finalMsgs := prov.calls[len(prov.calls)-1].msgs
	sawPR := false
	sawComment := false
	for _, m := range finalMsgs {
		if m.Role == RoleTool && m.Name == "list_prs" && strings.Contains(m.Content, "#42") {
			sawPR = true
		}
		if m.Role == RoleTool && m.Name == "list_issue_comments" && strings.Contains(m.Content, "LGTM") {
			sawComment = true
		}
	}
	if !sawPR || !sawComment {
		t.Errorf("final turn missing accumulated tool context (pr=%v, comment=%v)", sawPR, sawComment)
	}
}

// lastToolContent returns the most recent tool-result message body for
// the given tool name across all scripted calls. Used by the
// isolation test to assert on what each agent actually saw.
func lastToolContent(calls []scriptedCall, toolName string) string {
	var out string
	for _, c := range calls {
		for _, m := range c.msgs {
			if m.Role == RoleTool && m.Name == toolName {
				out = m.Content
			}
		}
	}
	return out
}

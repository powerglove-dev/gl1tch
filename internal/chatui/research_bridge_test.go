package chatui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/research"
)

// stubResearcher is a research.Researcher that returns a fixed Evidence
// without touching the network. Defined here so the chatui tests can
// drive a real research.Loop without depending on internal/executor.
type stubResearcher struct {
	name, describe, body string
	refs                 []string
}

func (s stubResearcher) Name() string     { return s.name }
func (s stubResearcher) Describe() string { return s.describe }

func (s stubResearcher) Gather(_ context.Context, _ research.ResearchQuery, _ research.EvidenceBundle) (research.Evidence, error) {
	return research.Evidence{
		Source: s.name,
		Title:  s.name + " evidence",
		Body:   s.body,
		Refs:   s.refs,
	}, nil
}

// stubLLM scripts the local model so the loop can run hermetically. The
// recognised stages mirror the prompts in internal/research/prompts.go.
func stubLLM(plan, draft, critique, judge string) research.LLMFn {
	return func(_ context.Context, prompt string) (string, error) {
		switch {
		case strings.Contains(prompt, "planning stage of a research loop"):
			return plan, nil
		case strings.Contains(prompt, "drafting stage of a research loop"):
			return draft, nil
		case strings.Contains(prompt, "critique stage of a research loop"):
			return critique, nil
		case strings.Contains(prompt, "judge stage of a research loop"):
			return judge, nil
		}
		return "", nil
	}
}

func newStubLoop(t *testing.T) *research.Loop {
	t.Helper()
	reg := research.NewRegistry()
	if err := reg.Register(stubResearcher{
		name:     "git-log",
		describe: "recent commits",
		body:     "abc1234 stokes: refactor router",
		refs:     []string{"abc1234"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(stubResearcher{
		name:     "github-prs",
		describe: "open prs",
		body:     "PR #412 refactor router (open)",
		refs:     []string{"https://example.com/pull/412"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	llm := stubLLM(
		`["git-log","github-prs"]`,
		"PR #412 refactors the router and matches commit abc1234.",
		`[{"text":"PR #412 is open","label":"grounded"}]`,
		"0.92",
	)
	return research.NewLoop(reg, llm).WithScoreOptions(research.ScoreOptions{
		Threshold:           0.7,
		SkipSelfConsistency: true,
		ShortCircuit:        false,
	})
}

func TestResearchResultToMessages_ProducesAnswerBundleAndScore(t *testing.T) {
	loop := newStubLoop(t)
	res, err := loop.Run(context.Background(), research.ResearchQuery{Question: "what's open?"}, research.DefaultBudget())
	if err != nil {
		t.Fatalf("loop.Run: %v", err)
	}
	msgs := ResearchResultToMessages(res)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (text + bundle + score), got %d", len(msgs))
	}
	if msgs[0].Type != MessageTypeText {
		t.Errorf("msg[0].Type = %s, want text", msgs[0].Type)
	}
	if msgs[1].Type != MessageTypeEvidenceBundle {
		t.Errorf("msg[1].Type = %s, want evidence_bundle", msgs[1].Type)
	}
	bundle, ok := msgs[1].Payload.(EvidenceBundlePayload)
	if !ok {
		t.Fatalf("msg[1].Payload is %T, want EvidenceBundlePayload", msgs[1].Payload)
	}
	if len(bundle.Items) != 2 {
		t.Errorf("bundle items = %d, want 2", len(bundle.Items))
	}
	if msgs[2].Type != MessageTypeScoreCard {
		t.Errorf("msg[2].Type = %s, want score_card", msgs[2].Type)
	}
}

func TestResearchSlashHandler_AppendsToStore(t *testing.T) {
	loop := newStubLoop(t)
	reg := NewSlashRegistry()
	if err := reg.Register(ResearchSlashHandler(loop)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	store := NewThreadStore()
	// User message that triggers the slash command.
	userMsg, err := store.Append(ChatMessage{
		Role:    RoleUser,
		Type:    MessageTypeText,
		Payload: TextPayload{Body: "/research what is open?"},
	})
	if err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if userMsg.ID == "" {
		t.Fatalf("user message ID empty")
	}

	msgs, err := reg.Dispatch(context.Background(), "/research what is open?", SlashScopeMain)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	for _, m := range msgs {
		if _, err := store.Append(m); err != nil {
			t.Fatalf("Append assistant msg: %v", err)
		}
	}
	main := store.MainScrollback()
	if len(main) < 4 { // user + text + bundle + score
		t.Errorf("MainScrollback len = %d, want ≥4", len(main))
	}
}

func TestResearchSlashHandler_EmptyQuestionUsageMessage(t *testing.T) {
	loop := newStubLoop(t)
	reg := NewSlashRegistry()
	_ = reg.Register(ResearchSlashHandler(loop))
	msgs, err := reg.Dispatch(context.Background(), "/research", SlashScopeMain)
	if err != nil {
		t.Fatalf("Dispatch empty: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Type != MessageTypeText {
		t.Fatalf("expected one usage text message, got %+v", msgs)
	}
	body := msgs[0].Payload.(TextPayload).Body
	if !strings.Contains(body, "Usage") {
		t.Errorf("usage message did not say Usage: %q", body)
	}
}

func TestSpawnDrillThread_SeedsEvidenceContext(t *testing.T) {
	store := NewThreadStore()
	parent, err := store.Append(ChatMessage{
		Role:    RoleAssistant,
		Type:    MessageTypeText,
		Payload: TextPayload{Body: "summary"},
	})
	if err != nil {
		t.Fatalf("Append parent: %v", err)
	}

	item := EvidenceBundleItem{
		Source: "github-prs",
		Title:  "open PR",
		Body:   "PR #412 refactors the router",
		Refs:   []string{"https://example.com/pull/412"},
	}
	thread, seed, err := SpawnDrillThread(store, parent.ID, item)
	if err != nil {
		t.Fatalf("SpawnDrillThread: %v", err)
	}
	if thread.State != ThreadOpen {
		t.Errorf("thread state = %s, want open", thread.State)
	}
	if seed.Role != RoleSystem {
		t.Errorf("seed message role = %s, want system", seed.Role)
	}
	body := seed.Payload.(TextPayload).Body
	for _, want := range []string{"github-prs", "PR #412", "https://example.com/pull/412"} {
		if !strings.Contains(body, want) {
			t.Errorf("seed body missing %q: %q", want, body)
		}
	}
	in := store.ThreadMessages(thread.ID)
	if len(in) != 1 {
		t.Errorf("thread messages = %d, want 1 (seed only)", len(in))
	}
}

func TestSpawnDrillThread_RejectsNestedParent(t *testing.T) {
	store := NewThreadStore()
	parent, _ := store.Append(ChatMessage{Role: RoleAssistant, Type: MessageTypeText, Payload: TextPayload{Body: "p"}})
	thread, _ := store.Spawn(parent.ID, ExpandInline)
	child, _ := store.Append(ChatMessage{ThreadID: thread.ID, Role: RoleUser, Type: MessageTypeText, Payload: TextPayload{Body: "in-thread"}})

	if _, _, err := SpawnDrillThread(store, child.ID, EvidenceBundleItem{Source: "x", Body: "y"}); !errors.Is(err, ErrNestingForbidden) {
		t.Errorf("nested SpawnDrillThread: got %v, want ErrNestingForbidden", err)
	}
}

// touch a time symbol so the import does not become unused
var _ = time.Now

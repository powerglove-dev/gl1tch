package chatui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/research"
)

// research_bridge.go is the chatui ↔ internal/research adapter. It exists so
// the chatui package can produce ChatMessage values from a research.Result
// without internal/research needing to know anything about chat or threads.
// This file is the only place a research import lives in chatui — every
// other piece of the package stays UI-only.
//
// Three things live here:
//
//   - ResearchResultToMessages: turns one Result into the assistant's
//     answer message + the trailing evidence_bundle widget. Used by both
//     the slash handler and any future direct caller (e.g. an attention
//     feed item that triggers a research drill).
//
//   - ResearchSlashHandler: a SlashHandler that runs the research loop in
//     the background, returns the result as ChatMessages, and is allowed
//     in both main and thread scopes (so the same /research command works
//     for "answer this in the main chat" and "answer this in a drill
//     thread that already has evidence context").
//
//   - SpawnDrillThread: the canonical chat-threads task 10 helper. Given
//     a parent message ID and one EvidenceBundleItem, it spawns a thread
//     under the parent and seeds it with a system message carrying the
//     evidence so subsequent assistant calls inside the thread can use
//     the evidence as context.

// ResearchResultToMessages converts one research.Result into the chat
// messages the assistant should append after a research call: the answer
// itself (text), the evidence bundle (widget), and the score card
// (widget) when the composite is non-zero.
//
// The function is pure — it does not append to a store or talk to any
// network. Callers append the returned messages to their ThreadStore.
func ResearchResultToMessages(result research.Result) []ChatMessage {
	now := time.Now()
	out := make([]ChatMessage, 0, 3)

	draft := strings.TrimSpace(result.Draft)
	if draft == "" {
		draft = "I don't have enough evidence to answer that."
	}
	out = append(out, ChatMessage{
		Role:      RoleAssistant,
		Type:      MessageTypeText,
		Payload:   TextPayload{Body: draft},
		CreatedAt: now,
	})

	if result.Bundle.Len() > 0 {
		bundle := EvidenceBundlePayload{
			Composite: result.Score.Composite,
		}
		for _, ev := range result.Bundle.Items {
			bundle.Items = append(bundle.Items, EvidenceBundleItem{
				Source: ev.Source,
				Title:  ev.Title,
				Body:   ev.Body,
				Refs:   append([]string(nil), ev.Refs...),
			})
		}
		out = append(out, ChatMessage{
			Role:      RoleAssistant,
			Type:      MessageTypeEvidenceBundle,
			Payload:   bundle,
			CreatedAt: now,
		})
	}

	// Score card: render only when there is a composite to show. The
	// renderer can sum signals from the bundle row count for the
	// sparkline; we only carry the current value here.
	if result.Score.Composite > 0 {
		out = append(out, ChatMessage{
			Role: RoleAssistant,
			Type: MessageTypeScoreCard,
			Payload: ScoreCardPayload{
				Metric: "confidence",
				Value:  result.Score.Composite,
			},
			CreatedAt: now,
		})
	}

	return out
}

// ResearchSlashHandler returns a SlashHandler bound to the supplied loop.
// The handler accepts the user's question as the raw arg string and
// returns the assistant's answer as ChatMessages.
//
// The handler is scope-agnostic: it can be invoked from main chat or from
// inside a thread. When invoked inside a thread, the caller is
// responsible for stamping the returned messages with that thread's ID
// before appending — the slash dispatcher does not own the store and
// cannot do that itself.
//
// Budget defaults to research.DefaultBudget(). Override by passing a
// custom budget closure to NewResearchSlashHandlerWithBudget.
func ResearchSlashHandler(loop *research.Loop) SlashHandler {
	return NewResearchSlashHandlerWithBudget(loop, func() research.Budget { return research.DefaultBudget() })
}

// NewResearchSlashHandlerWithBudget is the budget-aware variant. The
// budget closure is called once per dispatch so a caller can adapt the
// budget to current load (e.g. a smaller budget when the user is
// typing rapidly).
func NewResearchSlashHandlerWithBudget(loop *research.Loop, budgetFn func() research.Budget) SlashHandler {
	return SlashHandlerFunc{
		NameField:     "research",
		DescribeField: "Answer a question with grounded evidence from registered researchers",
		Fn: func(ctx context.Context, in SlashInvocation) ([]ChatMessage, error) {
			question := strings.TrimSpace(in.Raw)
			if question == "" {
				return []ChatMessage{{
					Role:      RoleAssistant,
					Type:      MessageTypeText,
					Payload:   TextPayload{Body: "Usage: /research <question>"},
					CreatedAt: time.Now(),
				}}, nil
			}
			if loop == nil {
				return nil, fmt.Errorf("chatui: /research handler has nil loop")
			}
			result, err := loop.Run(ctx, research.ResearchQuery{
				Question: question,
				Context:  in.Flags,
			}, budgetFn())
			if err != nil {
				return []ChatMessage{{
					Role:      RoleAssistant,
					Type:      MessageTypeText,
					Payload:   TextPayload{Body: fmt.Sprintf("research loop failed: %v", err)},
					CreatedAt: time.Now(),
				}}, nil
			}
			return ResearchResultToMessages(result), nil
		},
	}
}

// SpawnDrillThread is the chat-threads task 10 helper. It spawns a thread
// under the supplied parent message and seeds it with a system message
// that carries the picked evidence item as context. Subsequent /research
// calls inside the thread can include the evidence in the question via
// the in.Raw closure they use.
//
// Returns the new Thread plus the seed message that was inserted into
// it. Returns an error when the parent does not exist or when the parent
// is itself inside a thread (the no-nesting rule is enforced by Spawn).
func SpawnDrillThread(store *ThreadStore, parentMessageID string, item EvidenceBundleItem) (Thread, ChatMessage, error) {
	if store == nil {
		return Thread{}, ChatMessage{}, fmt.Errorf("chatui: SpawnDrillThread: nil store")
	}
	thread, err := store.Spawn(parentMessageID, ExpandInline)
	if err != nil {
		return Thread{}, ChatMessage{}, err
	}

	var b strings.Builder
	b.WriteString("Drilling into evidence: ")
	b.WriteString(item.Source)
	if item.Title != "" {
		b.WriteString(" — ")
		b.WriteString(item.Title)
	}
	b.WriteString("\n\n")
	if body := strings.TrimSpace(item.Body); body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	if len(item.Refs) > 0 {
		b.WriteString("\nrefs: ")
		b.WriteString(strings.Join(item.Refs, ", "))
		b.WriteString("\n")
	}

	seed, err := store.Append(ChatMessage{
		ThreadID:  thread.ID,
		Role:      RoleSystem,
		Type:      MessageTypeText,
		Payload:   TextPayload{Body: b.String()},
		CreatedAt: time.Now(),
	})
	if err != nil {
		return Thread{}, ChatMessage{}, fmt.Errorf("seed drill thread: %w", err)
	}
	return thread, seed, nil
}

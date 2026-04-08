package research

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// escalate.go is the paid-verifier escape hatch. The research loop is built
// to answer 95%+ of questions locally with qwen2.5:7b; for the remaining
// few — high-stakes summaries, security incidents, anything where the
// composite score lands stubbornly below the threshold — the operator can
// hand the assembled evidence bundle (NOT the raw question) to a paid
// model and ask it to verify or correct the local draft.
//
// Two design rules from the openspec proposal drive this file:
//
//   - "Escalation is opt-in per invocation and budgeted." A loop with
//     MaxPaidTokens=0 (the hard default) NEVER calls the verifier. The
//     loop logs the would-have-escalated decision and returns
//     ReasonBudgetExceeded as if escalation were not available, so the
//     accept/reject path stays observable in the brain stats engine.
//
//   - "The paid call is a judge, not a generator." The verifier prompt
//     receives the full evidence bundle and the local draft and is asked
//     to either confirm the draft or rewrite it grounded in the same
//     evidence. It is NEVER asked the original question with no context —
//     that would burn paid tokens to recreate work qwen2.5:7b already did.
//
// The Verifier interface is small and stable; the Claude-backed
// implementation lives in cmd/ask_research.go (which can import the
// executor manager) so this file does not have to.

// Verifier is the seam the loop uses to consult a paid model. Implementations
// must consume the supplied EvidenceBundle as the primary context and
// produce a verdict that either confirms the local draft or supersedes it.
type Verifier interface {
	// Name returns a stable identifier for the underlying model. The
	// loop logs this in the research_escalation event so brain stats can
	// attribute outcomes to a specific verifier.
	Name() string
	// Verify is called when the loop has finished its local iterations
	// and the composite score is below threshold. The implementation
	// returns either an error (loop continues with the local best
	// draft, escalation is logged as failed) or a Verdict with the
	// paid model's response and a token-usage count for budget
	// accounting.
	Verify(ctx context.Context, in VerifyInput) (Verdict, error)
}

// VerifyInput is the payload the loop hands to a verifier. The fields are
// snapshots of the loop's state at escalation time; the verifier must not
// retain references to them beyond the call.
type VerifyInput struct {
	Question string
	Draft    string
	Bundle   EvidenceBundle
	Score    Score
}

// Verdict is the verifier's response. Verbatim is true when the paid model
// confirmed the local draft without changes; in that case the loop returns
// the original draft with ReasonEscalated. When Verbatim is false, the
// loop returns Output as the new draft.
type Verdict struct {
	Output     string
	Verbatim   bool
	PaidTokens int
}

// VerifyPrompt builds the bundle-first prompt the loop sends to a verifier.
// The framing is "verify or correct" — the model is asked to either confirm
// the local draft or produce a new one grounded in the same evidence,
// never to re-answer the question from priors.
//
// The prompt deliberately does not name the local model. A verifier should
// not behave differently when told the draft came from a small model vs a
// large one — the question is "is this draft supported by the evidence,"
// not "is this draft worthy of a smaller model's effort."
func VerifyPrompt(in VerifyInput) string {
	var b strings.Builder
	b.WriteString("You are the verification stage of a research loop. A bounded local\n")
	b.WriteString("loop has gathered evidence and produced a draft answer. Your job is to\n")
	b.WriteString("either CONFIRM the draft (when every claim is supported by the evidence)\n")
	b.WriteString("or REWRITE it (using ONLY the same evidence) so every claim is supported.\n")
	b.WriteString("\n")
	b.WriteString("You MUST NOT use prior knowledge. You MUST NOT delegate to the user by\n")
	b.WriteString("saying \"you should run X\". If the evidence is insufficient, say so\n")
	b.WriteString("explicitly — do not guess.\n\n")

	b.WriteString("Output format:\n")
	b.WriteString("- If the draft is correct, output exactly: CONFIRM\n")
	b.WriteString("- Otherwise, output the corrected answer with no preamble.\n\n")

	b.WriteString("Question:\n")
	b.WriteString(strings.TrimSpace(in.Question))
	b.WriteString("\n\n")

	b.WriteString("Local draft:\n")
	b.WriteString(strings.TrimSpace(in.Draft))
	b.WriteString("\n\n")

	b.WriteString("Evidence:\n")
	if in.Bundle.Len() == 0 {
		b.WriteString("(no evidence was gathered)\n\n")
	} else {
		for i, ev := range in.Bundle.Items {
			fmt.Fprintf(&b, "[%d] source=%s\n", i+1, ev.Source)
			if ev.Title != "" {
				fmt.Fprintf(&b, "    title: %s\n", ev.Title)
			}
			if len(ev.Refs) > 0 {
				fmt.Fprintf(&b, "    refs: %s\n", strings.Join(ev.Refs, ", "))
			}
			body := strings.TrimSpace(ev.Body)
			if body != "" {
				b.WriteString("    body:\n")
				for _, line := range strings.Split(body, "\n") {
					fmt.Fprintf(&b, "      %s\n", line)
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("Output (CONFIRM or the corrected answer):\n")
	return b.String()
}

// ParseVerdict parses a verifier's raw response. A response that starts
// with "CONFIRM" (case-insensitive) returns Verbatim=true; anything else
// is treated as a rewritten draft. Empty responses return an error so the
// loop can fall back to the local best draft.
func ParseVerdict(raw string) (Verdict, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Verdict{}, errors.New("research: verifier returned empty output")
	}
	if strings.HasPrefix(strings.ToUpper(trimmed), "CONFIRM") {
		return Verdict{Verbatim: true, Output: trimmed}, nil
	}
	return Verdict{Output: trimmed}, nil
}

// EscalationDecision captures the loop's accept/reject choice for one
// escalation opportunity, plus the reason. It exists so the brain event
// log can record "we considered escalating but the budget was zero" as a
// distinct outcome from "we did not consider escalating because the local
// score cleared the threshold."
type EscalationDecision struct {
	WouldEscalate bool
	Reason        string
}

// shouldEscalate decides whether the loop should call the verifier. The
// rule is: composite below threshold AND iterations exhausted AND
// MaxPaidTokens > 0 AND verifier non-nil. Anything less is logged as a
// non-escalation reason for downstream learning.
func shouldEscalate(verifier Verifier, score Score, threshold float64, maxPaidTokens int, itersUsed, itersMax int) EscalationDecision {
	switch {
	case verifier == nil:
		return EscalationDecision{Reason: "no verifier configured"}
	case maxPaidTokens <= 0:
		return EscalationDecision{Reason: "MaxPaidTokens=0 (escalation off)"}
	case score.Composite >= threshold:
		return EscalationDecision{Reason: "local score already cleared threshold"}
	case itersUsed < itersMax:
		return EscalationDecision{Reason: "iterations remaining (refine first)"}
	}
	return EscalationDecision{WouldEscalate: true, Reason: "below threshold and no refine left"}
}

package research

import (
	"context"
	"fmt"
	"strings"

	"github.com/8op-org/gl1tch/internal/capability"
)

// LoopCapability adapts a research Loop into a capability.Capability so the
// assistant router can pick it for natural-language questions the same way
// it picks any other on-demand capability. The wrapper is intentionally
// thin: it carries no state of its own, it does not own the Loop's
// dependencies, and it imports capability without dragging the rest of the
// loop into the capability package.
//
// Why a wrapper at all (vs registering each researcher individually)? The
// research loop is one capability in the assistant's worldview — it gathers
// evidence from any number of researchers and produces a grounded draft,
// and the assistant should pick "research" once, not pick "github-prs" and
// then have to re-pick on the next iteration. Registering the loop as a
// single capability also keeps the assistant's pick prompt short, which is
// load-bearing for qwen2.5:7b.
type LoopCapability struct {
	// Loop is the underlying research loop. Must be non-nil.
	Loop *Loop
	// DescribeOverride lets a caller override the default capability
	// description shown to the assistant's pick model. Empty falls back
	// to a fixed description that mentions the canonical use cases (PR
	// review, evidence-grounded answers, anti-hallucination).
	DescribeOverride string
	// NameOverride lets a caller register the capability under a name
	// other than "research" (useful for tests, A/B routing experiments,
	// or running multiple loops side by side). Empty falls back to
	// "research".
	NameOverride string
	// Budget is the budget the wrapper uses for every Invoke call.
	// Defaults to DefaultBudget() when zero-valued.
	Budget Budget
}

// Manifest implements capability.Capability. The manifest is constructed
// fresh on every call so future tweaks to the description text do not
// require flushing a registry — but the manifest is cheap to build (a few
// strings) so this is fine.
func (lc *LoopCapability) Manifest() capability.Manifest {
	name := lc.NameOverride
	if name == "" {
		name = "research"
	}
	descr := lc.DescribeOverride
	if descr == "" {
		descr = "Answer a natural-language question by gathering grounded " +
			"evidence from registered researchers (git, github, workspace, " +
			"and pipeline-backed researchers) and writing an answer that " +
			"never invents identifiers. Use this for any question about " +
			"the current repository, its open PRs and issues, recent " +
			"commits, or anything else where the answer must be verifiable."
	}
	return capability.Manifest{
		Name:        name,
		Description: descr,
		Category:    "assistant.research",
		Trigger:     capability.Trigger{Mode: capability.TriggerOnDemand},
		Sink:        capability.Sink{Stream: true},
	}
}

// Invoke implements capability.Capability. It runs the Loop against the
// caller's question (taken from Input.Stdin), streams the draft as a single
// EventStream message, and stamps the per-source evidence titles + composite
// score onto a trailing EventStream block so the caller can render or log
// it without re-parsing the draft.
//
// Errors from the Loop become EventError events on the channel; the channel
// is closed in either case so the runner's drain loop terminates cleanly.
//
// The wrapper does not call any Doc-event sink: research output is
// inherently transient (it's an answer to one question), and persisting it
// is the brain event log's job, not the Doc indexer's.
func (lc *LoopCapability) Invoke(ctx context.Context, in capability.Input) (<-chan capability.Event, error) {
	if lc.Loop == nil {
		return nil, fmt.Errorf("research: LoopCapability has nil Loop")
	}
	question := strings.TrimSpace(in.Stdin)
	if question == "" {
		return nil, fmt.Errorf("research: LoopCapability requires a non-empty question on Input.Stdin")
	}
	budget := lc.Budget
	if budget.MaxIterations <= 0 {
		budget = DefaultBudget()
	}

	out := make(chan capability.Event, 4)
	go func() {
		defer close(out)
		result, err := lc.Loop.Run(ctx, ResearchQuery{
			Question: question,
			Context:  in.Vars,
		}, budget)
		if err != nil {
			out <- capability.Event{Kind: capability.EventError, Err: err}
			return
		}

		out <- capability.Event{
			Kind: capability.EventStream,
			Text: strings.TrimSpace(result.Draft) + "\n",
		}

		// Trailing block: bundle summary + score, separated from the
		// draft so a renderer can hide it or move it. Format mirrors
		// renderResearchResult in cmd/ask_research.go so the assistant
		// path and the ask path produce visually consistent output.
		var b strings.Builder
		if result.Bundle.Len() > 0 {
			b.WriteString("\n---\nevidence:\n")
			for i, ev := range result.Bundle.Items {
				title := ev.Title
				if title == "" {
					title = ev.Source
				}
				fmt.Fprintf(&b, "  [%d] %s — %s\n", i+1, ev.Source, title)
				if len(ev.Refs) > 0 {
					fmt.Fprintf(&b, "      refs: %s\n", strings.Join(ev.Refs, ", "))
				}
			}
		}
		fmt.Fprintf(&b, "\nconfidence: composite=%.2f reason=%s iterations=%d\n",
			result.Score.Composite, result.Reason, result.Iterations)
		out <- capability.Event{Kind: capability.EventStream, Text: b.String()}
	}()
	return out, nil
}

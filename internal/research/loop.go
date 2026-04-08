package research

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Loop is the research-loop driver: plan → gather → draft → critique → score.
// Refinement and escalation are still no-ops in this version (they belong to
// task groups 5–6); everything up to and including the composite confidence
// score is wired here.
//
// The driver is intentionally honest about its limits:
//   - Result.Reason is ReasonAccepted when the composite clears the
//     threshold, ReasonBudgetExceeded when it does not (no refine path yet),
//     and ReasonUnscored when the planner short-circuits with an empty plan.
//   - Refinement and escalation are no-ops in this version.
//
// What the driver does enforce, and where its value lives, is the prompts:
// the planner is forbidden from inventing researcher names, the drafter is
// forbidden from inventing identifiers, and the critique pass labels every
// claim as grounded / partial / ungrounded. Those rules are what stop the
// hallucinated-PRs failure mode the loop exists to prevent and what give the
// score signals something honest to measure.
type Loop struct {
	registry     *Registry
	llm          LLMFn
	model        string
	logger       *slog.Logger
	scoreOptions ScoreOptions
	events       EventSink
	verifier     Verifier
}

// NewLoop constructs a Loop. registry must be non-nil and populated; llm must
// be non-nil. logger may be nil — a default discarding logger is used so the
// loop never panics on missing telemetry.
func NewLoop(registry *Registry, llm LLMFn) *Loop {
	return &Loop{
		registry:     registry,
		llm:          llm,
		model:        DefaultLocalModel,
		logger:       slog.New(slog.NewTextHandler(discardWriter{}, nil)),
		scoreOptions: DefaultScoreOptions(),
		events:       nopSink{},
	}
}

// WithVerifier wires a paid verifier the loop consults when local
// iterations exhaust without clearing the threshold. Pass nil (or omit
// entirely) to keep escalation off — the loop honours MaxPaidTokens=0 as
// the escalation kill switch even when a verifier is wired.
func (l *Loop) WithVerifier(v Verifier) *Loop {
	l.verifier = v
	return l
}

// WithEventSink wires a brain-event sink the loop emits to once per
// iteration (and once per escalation, when that lands). Pass nil to
// disable telemetry; pass NewMemoryEventSink() in tests; pass
// NewFileEventSink("") in production to write JSONL to ~/.glitch.
func (l *Loop) WithEventSink(sink EventSink) *Loop {
	if sink == nil {
		l.events = nopSink{}
	} else {
		l.events = sink
	}
	return l
}

// WithScoreOptions overrides the loop's default score options. Returns the
// loop so the call chains with NewLoop. Pass an options value with
// SkipSelfConsistency=true to keep the smoke command's wallclock cost down
// while still computing the cheap signals.
func (l *Loop) WithScoreOptions(opts ScoreOptions) *Loop {
	l.scoreOptions = opts
	return l
}

// WithLogger attaches a structured logger. The loop emits one record per
// stage so a debugging caller can see plan output, picked researchers, draft
// length, and any per-researcher errors without having to instrument the
// stages themselves.
func (l *Loop) WithLogger(log *slog.Logger) *Loop {
	if log != nil {
		l.logger = log
	}
	return l
}

// Run executes one research call against the supplied budget. The driver
// runs plan → gather → draft → critique → score on every iteration; if the
// composite confidence score does not clear the threshold and the budget
// still has room, it refines (feeding the ungrounded claims from the
// critique back into the planner as additional evidence needs) and tries
// again. Errors from individual stages are wrapped with the stage name so
// the caller can tell whether the loop fell over in plan, gather, draft, or
// score.
//
// Budget enforcement is iteration-based and wallclock-based for v1. Token
// budgets are accepted on the Budget value but not enforced — the LLMFn
// seam does not currently carry usage information, and adding a stub-counter
// here would invent numbers that bear no relationship to what the model
// actually consumed. The brain event log will be the source of truth once
// the LLMFn surface grows a usage return value (task 7).
func (l *Loop) Run(ctx context.Context, q ResearchQuery, budget Budget) (Result, error) {
	if l.registry == nil {
		return Result{}, errors.New("research: Loop has nil registry")
	}
	if l.llm == nil {
		return Result{}, errors.New("research: Loop has nil llm")
	}
	if budget.MaxIterations <= 0 {
		budget = DefaultBudget()
	}

	deadline := time.Now().Add(budget.MaxWallclock)
	if budget.MaxWallclock <= 0 {
		deadline = time.Now().Add(60 * time.Second)
	}

	// best holds the highest-composite Result observed so far. The
	// budget-exceeded path returns it instead of throwing the work away,
	// so a partial answer is always better than nothing.
	var (
		best          Result
		haveBest      bool
		extraNeeds    []string // critique-derived asks the next plan should consider
		bundleSoFar   EvidenceBundle
		picksSoFar    = make(map[string]struct{})
		iter          int
	)

	for iter = 1; iter <= budget.MaxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return l.tryEscalate(ctx, q, best, haveBest, budget, iter-1), nil
		}
		if time.Now().After(deadline) {
			return l.tryEscalate(ctx, q, best, haveBest, budget, iter-1), nil
		}

		// Stage 1: plan. On refine iterations the planner sees the
		// original question PLUS the unsatisfied critique claims so it
		// can pick *additional* researchers — not the same ones again.
		picks, err := l.plan(ctx, augmentQuery(q, extraNeeds))
		if err != nil {
			return Result{}, fmt.Errorf("plan: %w", err)
		}
		// Drop any researchers we already gathered from in earlier
		// iterations; the loop never re-runs the same source twice on
		// the same question.
		picks = filterAlreadyPicked(picks, picksSoFar)
		l.logger.Info("research plan", "iter", iter, "question", q.Question, "picks", picks, "extra_needs", extraNeeds)

		// Empty plan on the first iteration → short-circuit with the
		// honest non-answer. Empty plan on a refine iteration → there
		// are no additional researchers to try; return the best draft
		// we have so far instead of looping on nothing.
		if len(picks) == 0 {
			if iter == 1 {
				draft, err := l.draft(ctx, q, EvidenceBundle{})
				if err != nil {
					return Result{}, fmt.Errorf("draft (empty plan): %w", err)
				}
				return Result{
					Query:      q,
					Draft:      draft,
					Bundle:     EvidenceBundle{},
					Reason:     ReasonUnscored,
					Iterations: iter,
				}, nil
			}
			return l.tryEscalate(ctx, q, best, haveBest, budget, iter-1), nil
		}

		// Stage 2: gather. Merges the new evidence into the running
		// bundle so the drafter sees everything gathered across
		// iterations, not just the current iteration's evidence.
		newBundle := l.gather(ctx, q, picks)
		for _, ev := range newBundle.Items {
			bundleSoFar.Add(ev)
		}
		for _, p := range picks {
			picksSoFar[p] = struct{}{}
		}
		l.logger.Info("research gathered", "iter", iter, "items", bundleSoFar.Len(), "sources", bundleSoFar.Sources())

		// Stage 3: draft.
		draft, err := l.draft(ctx, q, bundleSoFar)
		if err != nil {
			return Result{}, fmt.Errorf("draft: %w", err)
		}

		// Disabled-score path: skip critique/score, return Accepted
		// after one iteration (mirrors the pre-scoring v1 contract).
		if l.scoreOptions.Disabled {
			return Result{
				Query:      q,
				Draft:      draft,
				Bundle:     bundleSoFar,
				Reason:     ReasonAccepted,
				Iterations: iter,
			}, nil
		}

		// Stage 4: critique + score.
		score, crit, scoreErr := ComputeScore(ctx, ScoreInputs{
			Question: q.Question,
			Draft:    draft,
			Bundle:   bundleSoFar,
			LLM:      l.llm,
			Options:  l.scoreOptions,
			Redraft: func(ctx context.Context) (string, error) {
				return l.draft(ctx, q, bundleSoFar)
			},
		})
		if scoreErr != nil {
			l.logger.Warn("research score: stage failed", "iter", iter, "err", scoreErr.Error())
		}
		l.logger.Info("research scored",
			"iter", iter,
			"composite", score.Composite,
			"self_consistency", Float(score.SelfConsistency),
			"evidence_coverage", Float(score.EvidenceCoverage),
			"cross_capability", Float(score.CrossCapabilityAgree),
			"judge", Float(score.JudgeScore),
		)

		current := Result{
			Query:      q,
			Draft:      draft,
			Bundle:     bundleSoFar,
			Score:      score,
			Reason:     ReasonAccepted,
			Iterations: iter,
		}

		// Emit per-iteration events so the brain stats engine has a
		// permanent record of which signals fired and which composite
		// emerged. Reason at this point is provisional (Accepted vs
		// BudgetExceeded is decided after the threshold check below);
		// downstream queries that care about the final disposition
		// should look at the last event for the QueryID.
		emitAttempt(l.events, q, iter, score, bundleSoFar, ReasonAccepted)

		// Always remember the best Result so far — refine iterations
		// might score lower (e.g. if a noisy researcher pollutes the
		// bundle), and budget-exceeded should return the highest-
		// confidence draft, not the latest.
		if !haveBest || score.Composite > best.Score.Composite {
			best = current
			haveBest = true
		}

		// Accept path: composite cleared the bar. Return immediately;
		// the budget is not consumed beyond this point.
		if score.Composite >= l.scoreOptions.Threshold {
			return current, nil
		}

		// Refine path: extract the critique's ungrounded claims and
		// feed them back into the next plan as additional evidence
		// needs. If there are none (every claim was grounded but the
		// score still didn't clear the bar — usually because cross-
		// capability is 0.4 with one source), there is nothing useful
		// to refine on, so we exit early instead of looping uselessly.
		extraNeeds = ungroundedClaims(crit)
		if len(extraNeeds) == 0 {
			return l.tryEscalate(ctx, q, best, haveBest, budget, iter), nil
		}
	}

	// Hit MaxIterations without clearing the threshold. Consider
	// escalation before returning the best-effort answer; if the loop
	// is configured for it, hand the bundle to the paid verifier and
	// return its verdict instead.
	return l.tryEscalate(ctx, q, best, haveBest, budget, iter-1), nil
}

// tryEscalate consults the configured Verifier when the loop has
// exhausted its local budget. The escalation gate (composite < threshold,
// MaxPaidTokens > 0, verifier non-nil) is enforced by shouldEscalate; this
// function is the wiring that calls Verify, parses the verdict, and
// folds the result back into the returned Result.
//
// On any failure (verifier nil, gate refused, Verify error, ParseVerdict
// error) tryEscalate returns the budget-exceeded best-effort answer with
// the original score breakdown intact, so the caller never loses the
// local work to a verifier outage.
func (l *Loop) tryEscalate(ctx context.Context, q ResearchQuery, best Result, haveBest bool, budget Budget, itersUsed int) Result {
	fallback := l.budgetExceeded(best, haveBest, q, itersUsed)

	if !haveBest {
		return fallback
	}

	decision := shouldEscalate(l.verifier, best.Score, l.scoreOptions.Threshold, budget.MaxPaidTokens, itersUsed, budget.MaxIterations)
	l.logger.Info("research escalation decision",
		"would_escalate", decision.WouldEscalate,
		"reason", decision.Reason,
		"composite", best.Score.Composite,
	)
	if !decision.WouldEscalate {
		return fallback
	}

	verdict, err := l.verifier.Verify(ctx, VerifyInput{
		Question: q.Question,
		Draft:    best.Draft,
		Bundle:   best.Bundle,
		Score:    best.Score,
	})
	if err != nil {
		l.logger.Warn("research escalation: verifier error", "err", err.Error())
		l.emitEscalationEvent(q, l.verifier.Name(), 0, "error", best.Score)
		return fallback
	}

	out := best
	out.Reason = ReasonEscalated
	if !verdict.Verbatim {
		out.Draft = verdict.Output
	}
	verdictLabel := "rewrite"
	if verdict.Verbatim {
		verdictLabel = "confirm"
	}
	l.emitEscalationEvent(q, l.verifier.Name(), verdict.PaidTokens, verdictLabel, best.Score)
	return out
}

// emitEscalationEvent writes one EventTypeEscalation record to the brain
// event sink. Best-effort: any sink error is silently dropped because the
// caller is already in the budget-exceeded path and a telemetry failure
// must not regress the user-visible answer.
func (l *Loop) emitEscalationEvent(q ResearchQuery, paidModel string, paidTokens int, verdict string, score Score) {
	if l.events == nil {
		return
	}
	_ = l.events.Emit(Event{
		Type:       EventTypeEscalation,
		Timestamp:  time.Now().Format(time.RFC3339),
		QueryID:    q.ID,
		Question:   q.Question,
		Reason:     ReasonEscalated,
		Score:      score,
		PaidModel:  paidModel,
		PaidTokens: paidTokens,
		Verdict:    verdict,
	})
}

// budgetExceeded constructs the Result returned when the loop runs out of
// budget without clearing the threshold. It returns the best Result
// observed so far re-stamped with ReasonBudgetExceeded; if no iteration
// produced a Result at all (e.g. context cancelled before iter 1) it returns
// a placeholder Result with the honest "no draft" answer.
func (l *Loop) budgetExceeded(best Result, haveBest bool, q ResearchQuery, iters int) Result {
	if !haveBest {
		return Result{
			Query:      q,
			Draft:      "I ran out of time before I could gather enough evidence to answer that.",
			Reason:     ReasonBudgetExceeded,
			Iterations: iters,
		}
	}
	out := best
	out.Reason = ReasonBudgetExceeded
	return out
}

// augmentQuery returns a copy of q with extraNeeds appended to the question
// as additional evidence-gathering hints. The planner sees the original ask
// plus a "still missing:" suffix; the planner prompt rules tell it to pick
// researchers that can supply that missing evidence.
func augmentQuery(q ResearchQuery, extraNeeds []string) ResearchQuery {
	if len(extraNeeds) == 0 {
		return q
	}
	out := q
	var b strings.Builder
	b.WriteString(strings.TrimSpace(q.Question))
	b.WriteString("\n\nStill missing evidence for these claims (pick researchers that can supply them):\n")
	for _, need := range extraNeeds {
		b.WriteString("- ")
		b.WriteString(need)
		b.WriteString("\n")
	}
	out.Question = b.String()
	return out
}

// filterAlreadyPicked removes researcher names from picks that have already
// been gathered from in an earlier iteration. The loop's contract is that
// the same researcher is never invoked twice for the same question — refine
// must broaden, not retry.
func filterAlreadyPicked(picks []string, already map[string]struct{}) []string {
	out := make([]string, 0, len(picks))
	for _, p := range picks {
		if _, dup := already[p]; dup {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ungroundedClaims returns the claim texts the critique labelled as
// LabelUngrounded or LabelPartial — the claims the loop should try to
// gather more evidence for on the next iteration.
func ungroundedClaims(c Critique) []string {
	var out []string
	for _, claim := range c.Claims {
		if claim.Label == LabelUngrounded || claim.Label == LabelPartial {
			out = append(out, claim.Text)
		}
	}
	return out
}

// plan asks the local model to pick researcher names and validates the
// result against the registry. Names not in the registry are dropped with a
// warning; this is the "validate against registry before dispatch" rule from
// the spec.
func (l *Loop) plan(ctx context.Context, q ResearchQuery) ([]string, error) {
	prompt := PlanPrompt(q.Question, l.registry.List())
	raw, err := l.llm(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	picks, err := ParsePlan(raw)
	if err != nil {
		return nil, err
	}
	return l.validatePicks(picks), nil
}

// validatePicks drops any researcher names that are not registered. Logs the
// dropped names so a debugging operator can see whether the planner is
// hallucinating.
func (l *Loop) validatePicks(picks []string) []string {
	out := make([]string, 0, len(picks))
	for _, name := range picks {
		if _, ok := l.registry.Lookup(name); ok {
			out = append(out, name)
			continue
		}
		l.logger.Warn("research plan: dropping unknown researcher", "name", name)
	}
	return out
}

// gather invokes the picked researchers in parallel and aggregates their
// evidence into a single bundle. A researcher error is logged and skipped;
// the bundle continues with whatever other researchers produced. This is
// the partial-bundle rule from the spec.
func (l *Loop) gather(ctx context.Context, q ResearchQuery, picks []string) EvidenceBundle {
	type result struct {
		idx int
		ev  Evidence
		err error
	}
	results := make([]result, len(picks))
	var wg sync.WaitGroup
	for i, name := range picks {
		researcher, ok := l.registry.Lookup(name)
		if !ok {
			results[i] = result{idx: i, err: fmt.Errorf("registry: %q vanished", name)}
			continue
		}
		wg.Add(1)
		go func(i int, r Researcher) {
			defer wg.Done()
			ev, err := r.Gather(ctx, q, EvidenceBundle{})
			results[i] = result{idx: i, ev: ev, err: err}
		}(i, researcher)
	}
	wg.Wait()

	var bundle EvidenceBundle
	for _, r := range results {
		if r.err != nil {
			l.logger.Warn("research gather: researcher error",
				"researcher", picks[r.idx], "err", r.err.Error())
			// If the researcher returned a partial body alongside its
			// error, keep it — partial evidence is better than none.
			if r.ev.Source != "" || r.ev.Body != "" {
				bundle.Add(r.ev)
			}
			continue
		}
		bundle.Add(r.ev)
	}
	return bundle
}

// draft asks the local model to write the answer using only the bundle. The
// draft prompt is the second half of the lie-prevention contract.
func (l *Loop) draft(ctx context.Context, q ResearchQuery, bundle EvidenceBundle) (string, error) {
	prompt := DraftPrompt(q.Question, bundle)
	out, err := l.llm(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("llm: %w", err)
	}
	return out, nil
}

// discardWriter is a no-op io.Writer for the default slog handler. Defined
// here to avoid pulling in io/ioutil or os.Stderr by accident.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

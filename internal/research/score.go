package research

import (
	"context"
	"fmt"
	"strings"
)

// score.go implements the confidence-scoring stage of the research loop. It
// is the first half of "task group 4" in openspec/changes/glitch-research-loop:
// the four signals (SelfConsistency, EvidenceCoverage, CrossCapabilityAgree,
// JudgePass), the equal-weight Composite, and the cheap-signal-first ordering
// the loop uses to short-circuit when a draft is decisively above or below the
// accept threshold.
//
// Two design rules from the project memory drive this file:
//
//   - "AI-first, nothing hardcoded." Wherever a signal needs to *judge*
//     anything (is this claim supported, do these drafts agree, does the
//     draft answer the question), the work goes through the LLMFn seam, not
//     a regex or keyword table. The only structural signal is
//     CrossCapabilityAgree, which is a count of distinct researcher sources
//     in the bundle — that is a fact about the gather stage, not a
//     classification of evidence content.
//
//   - "qwen2.5:7b is the hard default." Every LLM call here goes through the
//     same LLMFn the rest of the loop uses, so changing the model in one
//     place changes it everywhere. The Score* helpers do not import
//     internal/executor or internal/pipeline; they take an LLMFn so tests
//     can drive them with deterministic stubs.
//
// The composite is an equal-weight average of the per-signal Float() values.
// That is intentional for v1 — the proposal explicitly says equal weights are
// fine because the *floor* (must have evidence at all) is what's broken right
// now, not the ceiling (which signal predicts accept). Learned weights live
// downstream of the brain event log this code populates.

// CritiqueLabel is one of three per-claim grounding verdicts the critique
// stage emits when scoring evidence coverage. The labels are exact strings so
// the LLM prompt can require them verbatim and the parser can switch on them
// without normalisation.
type CritiqueLabel string

const (
	// LabelGrounded means every specific identifier in the claim
	// (PR number, file path, commit SHA, date, name) appears verbatim
	// in the evidence bundle.
	LabelGrounded CritiqueLabel = "grounded"
	// LabelPartial means some, but not all, identifiers in the claim
	// appear in the bundle. Partial claims contribute 0.5 to the
	// EvidenceCoverage signal.
	LabelPartial CritiqueLabel = "partial"
	// LabelUngrounded means the claim contains identifiers that do not
	// appear in the bundle, OR the claim has no evidence basis at all.
	// Ungrounded claims contribute 0 and are the failure mode the loop
	// exists to prevent (see project_research_loop_negative_example).
	LabelUngrounded CritiqueLabel = "ungrounded"
)

// Critique is the structured output of the critique stage: one verdict per
// extracted claim. The Loop holds onto this between draft and score so
// EvidenceCoverage can sum the labels without re-running the critique LLM
// call.
type Critique struct {
	Claims []CritiqueClaim `json:"claims"`
}

// CritiqueClaim is one labelled claim from a draft.
type CritiqueClaim struct {
	Text  string        `json:"text"`
	Label CritiqueLabel `json:"label"`
}

// CritiquePrompt builds the prompt the critique stage sends to the LLM. The
// model is asked to (1) extract the claims from the draft, (2) check each
// against the evidence, and (3) emit a JSON array of {text, label} objects.
//
// The prompt is intentionally narrow: it does not ask for explanations,
// rewrites, or scores. Asking small models to "extract claims" with a free-
// form schema reliably produces verbose prose; pinning the output to a flat
// array of objects is the only shape that round-trips through ParseCritique
// with high reliability against qwen2.5:7b.
func CritiquePrompt(draft string, bundle EvidenceBundle) string {
	var b strings.Builder
	b.WriteString("You are the critique stage of a research loop. Your job is to extract\n")
	b.WriteString("the factual claims from a draft answer and label each one against the\n")
	b.WriteString("evidence the loop gathered. You do NOT rewrite the draft. You do NOT\n")
	b.WriteString("answer the original question. You only extract and label.\n\n")

	b.WriteString("Draft:\n")
	b.WriteString(strings.TrimSpace(draft))
	b.WriteString("\n\n")

	b.WriteString("Evidence:\n")
	if bundle.Len() == 0 {
		b.WriteString("(no evidence was gathered)\n\n")
	} else {
		for i, ev := range bundle.Items {
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

	b.WriteString("Labels (use exactly one per claim):\n")
	b.WriteString("- \"grounded\":   every specific identifier in the claim (PR number,\n")
	b.WriteString("                file path, commit SHA, date, name, URL) appears\n")
	b.WriteString("                verbatim in the evidence above.\n")
	b.WriteString("- \"partial\":    some identifiers appear and some do not, OR the\n")
	b.WriteString("                claim is roughly supported but not exactly.\n")
	b.WriteString("- \"ungrounded\": the claim contains identifiers that do not appear\n")
	b.WriteString("                in the evidence, OR the claim has no evidence basis.\n\n")

	b.WriteString("Output ONLY a JSON array of objects with this shape, no prose, no markdown:\n")
	b.WriteString("[{\"text\": \"...\", \"label\": \"grounded|partial|ungrounded\"}, ...]\n\n")
	b.WriteString("Output (JSON array only):\n")
	return b.String()
}

// ParseCritique extracts a Critique from a critique-stage LLM output. Like
// ParsePlan it is tolerant of leading or trailing prose by scanning for the
// first '[' and matching brackets — small models occasionally preface their
// JSON despite being asked not to.
func ParseCritique(raw string) (Critique, error) {
	start := strings.Index(raw, "[")
	if start < 0 {
		return Critique{}, fmt.Errorf("research: critique output has no JSON array: %q", truncate(raw, 200))
	}
	depth := 0
	inString := false
	escaped := false
	end := -1
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		return Critique{}, fmt.Errorf("research: critique output has unbalanced JSON array: %q", truncate(raw, 200))
	}

	var raws []struct {
		Text  string `json:"text"`
		Label string `json:"label"`
	}
	if err := jsonUnmarshalStrict(raw[start:end], &raws); err != nil {
		return Critique{}, fmt.Errorf("research: critique output is not a {text,label} array: %v", err)
	}

	out := Critique{Claims: make([]CritiqueClaim, 0, len(raws))}
	for _, r := range raws {
		text := strings.TrimSpace(r.Text)
		if text == "" {
			continue
		}
		label := normaliseLabel(r.Label)
		out.Claims = append(out.Claims, CritiqueClaim{Text: text, Label: label})
	}
	return out, nil
}

// normaliseLabel maps any case-insensitive variant of the three labels to its
// canonical CritiqueLabel value. Unrecognised labels become LabelUngrounded —
// "I don't know what that means" is a stricter floor than "I'll trust it,"
// which is the right default for the failure mode this whole loop exists to
// prevent.
func normaliseLabel(s string) CritiqueLabel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(LabelGrounded):
		return LabelGrounded
	case string(LabelPartial):
		return LabelPartial
	case string(LabelUngrounded):
		return LabelUngrounded
	default:
		return LabelUngrounded
	}
}

// EvidenceCoverage computes the evidence-coverage signal from a Critique.
// The formula is the one named in the spec:
//
//	(grounded + 0.5 * partial) / total_claims
//
// An empty critique returns 0 — a draft with no extractable claims has no
// evidence to cover, which is indistinguishable from a draft that ignored the
// evidence entirely. The loop's empty-bundle path returns ReasonUnscored
// before this function is called, so the zero here only fires for the
// genuinely-degenerate "model returned no parseable claims" case.
func EvidenceCoverage(c Critique) float64 {
	if len(c.Claims) == 0 {
		return 0
	}
	var sum float64
	for _, claim := range c.Claims {
		switch claim.Label {
		case LabelGrounded:
			sum += 1.0
		case LabelPartial:
			sum += 0.5
		}
	}
	return sum / float64(len(c.Claims))
}

// CrossCapabilityAgree returns the cross-capability agreement signal: how
// many distinct researcher sources contributed to the bundle. The spec
// defines this as "1.0 when ≥2 researchers independently support the
// conclusion, ≤0.4 otherwise" — implemented here as a structural count of
// distinct sources, which is the closest non-LLM proxy for "independent
// support" the loop can compute without reading every body byte.
//
// The signal is intentionally not LLM-derived. Source count is a property of
// the gather stage, not a classification of evidence content; the AI-first
// rule is about pushing classification to the LLM, not pushing arithmetic to
// the LLM. A future iteration may add a true semantic-agreement check (does
// source A's body actually corroborate source B's body?) and weight the two
// signals together.
func CrossCapabilityAgree(bundle EvidenceBundle) float64 {
	switch n := len(bundle.Sources()); {
	case n >= 2:
		return 1.0
	case n == 1:
		return 0.4
	default:
		return 0.0
	}
}

// JudgePrompt builds the prompt for the rubric-based judge pass. The judge
// is asked to read the question, the draft, and the evidence, and return a
// single numeric score in [0,1] reflecting how well the draft answers the
// question using only the evidence. Like the critique prompt it forbids
// commentary so the parser only has to find one number.
func JudgePrompt(question, draft string, bundle EvidenceBundle) string {
	var b strings.Builder
	b.WriteString("You are the judge stage of a research loop. Read the question, the\n")
	b.WriteString("draft answer, and the evidence the loop gathered, and return ONE number\n")
	b.WriteString("between 0.0 and 1.0 representing how well the draft answers the question\n")
	b.WriteString("using only the evidence. Do NOT explain. Do NOT add prose. Output the\n")
	b.WriteString("number on a line by itself.\n\n")

	b.WriteString("Rubric:\n")
	b.WriteString("- 1.0: the draft answers the question completely and every specific claim\n")
	b.WriteString("       is supported by the evidence.\n")
	b.WriteString("- 0.7: the draft answers the question, with one or two minor claims that\n")
	b.WriteString("       are not directly supported.\n")
	b.WriteString("- 0.4: the draft partially answers the question or contains several\n")
	b.WriteString("       unsupported claims.\n")
	b.WriteString("- 0.1: the draft fails to address the question or invents identifiers.\n")
	b.WriteString("- 0.0: the draft is empty, irrelevant, or refuses to answer.\n\n")

	b.WriteString("Question:\n")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\n")

	b.WriteString("Draft:\n")
	b.WriteString(strings.TrimSpace(draft))
	b.WriteString("\n\n")

	b.WriteString("Evidence:\n")
	if bundle.Len() == 0 {
		b.WriteString("(no evidence was gathered)\n\n")
	} else {
		for i, ev := range bundle.Items {
			fmt.Fprintf(&b, "[%d] source=%s title=%s\n", i+1, ev.Source, ev.Title)
		}
		b.WriteString("\n")
	}

	b.WriteString("Score (single number, 0.0 to 1.0):\n")
	return b.String()
}

// ParseJudgeScore extracts the numeric score from a judge-stage LLM output.
// It scans for the first decimal-looking token and clamps the result to
// [0,1]. Anything outside that range is treated as a parse failure (returns
// 0, error) so the caller can decide whether to retry or skip the signal.
func ParseJudgeScore(raw string) (float64, error) {
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c >= '0' && c <= '9') || c == '.' {
			end := i
			for end < len(raw) {
				cc := raw[end]
				if (cc >= '0' && cc <= '9') || cc == '.' {
					end++
					continue
				}
				break
			}
			var v float64
			n, err := fmt.Sscanf(raw[i:end], "%f", &v)
			if err == nil && n == 1 {
				if v < 0 || v > 1 {
					return 0, fmt.Errorf("research: judge score out of range: %v", v)
				}
				return v, nil
			}
			i = end
		}
	}
	return 0, fmt.Errorf("research: judge output has no numeric score: %q", truncate(raw, 200))
}

// JudgePass runs the judge prompt through the given LLMFn and returns the
// parsed score. Errors from the LLM call or the parser propagate so the
// caller can drop the signal (set the *float64 to nil in the Score) without
// failing the whole loop.
func JudgePass(ctx context.Context, llm LLMFn, question, draft string, bundle EvidenceBundle) (float64, error) {
	if llm == nil {
		return 0, fmt.Errorf("research: JudgePass: nil LLMFn")
	}
	out, err := llm(ctx, JudgePrompt(question, draft, bundle))
	if err != nil {
		return 0, fmt.Errorf("research: judge llm: %w", err)
	}
	return ParseJudgeScore(out)
}

// SelfConsistencyPrompt asks the LLM to compare the original draft to N-1
// alternative drafts of the same question and return a single
// agreement-of-conclusions score in [0,1]. The drafts themselves are
// generated by the loop's draft stage and passed in here — this function
// only owns the comparison prompt.
func SelfConsistencyPrompt(question, original string, alternatives []string) string {
	var b strings.Builder
	b.WriteString("You are the self-consistency stage of a research loop. You will see one\n")
	b.WriteString("question and several drafts of an answer. Return ONE number between 0.0\n")
	b.WriteString("and 1.0 representing how much the drafts AGREE on the conclusion.\n")
	b.WriteString("- 1.0 means every draft draws the same conclusion (wording may differ).\n")
	b.WriteString("- 0.0 means the drafts contradict each other.\n")
	b.WriteString("Do NOT explain. Do NOT add prose. Output the number on a line by itself.\n\n")

	b.WriteString("Question:\n")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\n")

	b.WriteString("Original draft:\n")
	b.WriteString(strings.TrimSpace(original))
	b.WriteString("\n\n")

	for i, alt := range alternatives {
		fmt.Fprintf(&b, "Alternative %d:\n%s\n\n", i+1, strings.TrimSpace(alt))
	}

	b.WriteString("Agreement score (single number, 0.0 to 1.0):\n")
	return b.String()
}

// SelfConsistency re-samples the draft N times via redraft and asks the LLM
// to score the resulting drafts for agreement. N must be at least 2 (one
// original plus one alternative); a smaller N returns 0 with an error so the
// caller can drop the signal.
//
// redraft is a closure the loop supplies. It must produce one fresh draft for
// the same query, and is expected to use a higher temperature than the
// production draft stage so the alternatives actually differ. Keeping
// temperature out of LLMFn lets the loop choose the redraft strategy without
// changing the seam every other stage uses.
func SelfConsistency(
	ctx context.Context,
	llm LLMFn,
	question, original string,
	n int,
	redraft func(ctx context.Context) (string, error),
) (float64, error) {
	if n < 2 {
		return 0, fmt.Errorf("research: SelfConsistency: n must be ≥2, got %d", n)
	}
	if llm == nil {
		return 0, fmt.Errorf("research: SelfConsistency: nil LLMFn")
	}
	if redraft == nil {
		return 0, fmt.Errorf("research: SelfConsistency: nil redraft fn")
	}

	alternatives := make([]string, 0, n-1)
	for i := 0; i < n-1; i++ {
		alt, err := redraft(ctx)
		if err != nil {
			return 0, fmt.Errorf("research: SelfConsistency: redraft %d: %w", i+1, err)
		}
		alternatives = append(alternatives, alt)
	}

	raw, err := llm(ctx, SelfConsistencyPrompt(question, original, alternatives))
	if err != nil {
		return 0, fmt.Errorf("research: SelfConsistency: llm: %w", err)
	}
	return ParseJudgeScore(raw)
}

// Composite folds the per-signal scores in s into a single scalar in [0,1]
// using equal weights. Missing signals (nil pointers) contribute 0 to the
// numerator and 0 to the denominator — they are skipped, not penalised, so
// dropping a signal does not pull the composite down.
//
// This is the v1 weighting per the proposal: the floor (any evidence at all)
// is what's broken right now, not the ceiling (which signal predicts accept).
// Learned weights live downstream of the brain event log this code populates.
func Composite(s Score) float64 {
	var sum float64
	var n int
	if s.SelfConsistency != nil {
		sum += *s.SelfConsistency
		n++
	}
	if s.EvidenceCoverage != nil {
		sum += *s.EvidenceCoverage
		n++
	}
	if s.CrossCapabilityAgree != nil {
		sum += *s.CrossCapabilityAgree
		n++
	}
	if s.JudgeScore != nil {
		sum += *s.JudgeScore
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// ScoreOptions controls which signals the loop computes during the score
// stage. The defaults match the cheap-signal-first ordering from the spec:
// the structural CrossCapabilityAgree runs first (free), then the critique-
// derived EvidenceCoverage (one LLM call), then the rubric JudgePass (one
// LLM call), and finally SelfConsistency (N+1 LLM calls — the most expensive
// signal). The Threshold and ShortCircuit fields let the loop bail out early
// when the running average is decisively above or below the accept bar
// without paying for the more expensive signals.
type ScoreOptions struct {
	// Disabled bypasses the score stage entirely. The loop returns
	// ReasonAccepted with a zero-valued Score, matching the pre-scoring
	// v1 contract. Tests that predate scoring set this to keep their
	// LLM scripts short; production callers should leave it false.
	Disabled bool
	// SkipSelfConsistency disables the most expensive signal entirely.
	// Defaults to false; tests and the v1 smoke command set it to true to
	// keep wallclock costs down.
	SkipSelfConsistency bool
	// SelfConsistencyN is the number of total drafts (original + N-1
	// alternatives) the SelfConsistency signal compares. Defaults to 3.
	SelfConsistencyN int
	// Threshold is the accept bar in [0,1]. When ShortCircuit is true the
	// scorer stops as soon as the running average makes the threshold
	// unreachable in either direction.
	Threshold float64
	// ShortCircuit enables the cheap-signal-first short-circuit. When
	// false the scorer computes every enabled signal regardless of the
	// running average.
	ShortCircuit bool
}

// DefaultScoreOptions returns the conservative defaults the loop uses when
// no caller has supplied its own. Self-consistency stays on, N=3, threshold
// 0.7, short-circuit on. These are the values from the openspec proposal's
// "task group 4" defaults section.
func DefaultScoreOptions() ScoreOptions {
	return ScoreOptions{
		SelfConsistencyN: 3,
		Threshold:        0.7,
		ShortCircuit:     true,
	}
}

// ScoreInputs bundles every value the score stage needs into one struct so
// the loop can call ComputeScore once at the end of the draft stage without
// passing seven positional args.
type ScoreInputs struct {
	Question  string
	Draft     string
	Bundle    EvidenceBundle
	LLM       LLMFn
	Options   ScoreOptions
	// Redraft is the closure SelfConsistency uses to produce alternative
	// drafts. The loop's score stage builds it from the loop's own draft
	// stage so the score path does not have to know how the original draft
	// was constructed. May be nil when SkipSelfConsistency is true.
	Redraft func(ctx context.Context) (string, error)
}

// ComputeScore runs the score stage and returns a fully-populated Score plus
// the per-claim Critique (so the caller can log it without re-running the
// critique LLM call). It honours the cheap-signal-first ordering and the
// short-circuit rule on Options.
//
// Errors from individual signals do NOT fail ComputeScore — a failed signal
// becomes a nil pointer in the returned Score, the loop logs the error via
// the slog handler, and Composite() skips it. This is the partial-bundle
// rule applied to scoring: a degraded score is more useful than no score.
func ComputeScore(ctx context.Context, in ScoreInputs) (Score, Critique, error) {
	out := Score{}
	var crit Critique

	// Signal 1: cross-capability agreement (free, structural).
	cca := CrossCapabilityAgree(in.Bundle)
	out.CrossCapabilityAgree = Ptr(cca)

	if in.Options.ShortCircuit && unreachable(out, in.Options.Threshold, 3) {
		out.Composite = Composite(out)
		return out, crit, nil
	}

	// Signal 2: evidence coverage (one LLM call via critique).
	if in.LLM != nil {
		raw, err := in.LLM(ctx, CritiquePrompt(in.Draft, in.Bundle))
		if err == nil {
			parsed, perr := ParseCritique(raw)
			if perr == nil {
				crit = parsed
				ec := EvidenceCoverage(parsed)
				out.EvidenceCoverage = Ptr(ec)
			}
		}
	}

	if in.Options.ShortCircuit && unreachable(out, in.Options.Threshold, 2) {
		out.Composite = Composite(out)
		return out, crit, nil
	}

	// Signal 3: judge pass (one LLM call).
	if in.LLM != nil {
		js, err := JudgePass(ctx, in.LLM, in.Question, in.Draft, in.Bundle)
		if err == nil {
			out.JudgeScore = Ptr(js)
		}
	}

	if in.Options.ShortCircuit && unreachable(out, in.Options.Threshold, 1) {
		out.Composite = Composite(out)
		return out, crit, nil
	}

	// Signal 4: self-consistency (N+1 LLM calls — most expensive).
	if !in.Options.SkipSelfConsistency && in.LLM != nil && in.Redraft != nil {
		n := in.Options.SelfConsistencyN
		if n < 2 {
			n = 2
		}
		sc, err := SelfConsistency(ctx, in.LLM, in.Question, in.Draft, n, in.Redraft)
		if err == nil {
			out.SelfConsistency = Ptr(sc)
		}
	}

	out.Composite = Composite(out)
	return out, crit, nil
}

// unreachable returns true when no possible value of the remaining signals
// can pull the running composite across the threshold from its current side.
// remainingSignals is the number of signals not yet computed; the caller
// passes the count so this helper does not have to know the schedule.
//
// The check is two-sided: if the running average is decisively above the
// threshold AND every remaining signal would have to score 0 to pull it
// below, we accept early. Mirror logic for decisive rejection.
func unreachable(s Score, threshold float64, remainingSignals int) bool {
	// Count computed signals and their sum.
	var sum float64
	var n int
	for _, p := range []*float64{s.SelfConsistency, s.EvidenceCoverage, s.CrossCapabilityAgree, s.JudgeScore} {
		if p != nil {
			sum += *p
			n++
		}
	}
	if n == 0 {
		return false
	}
	total := float64(n + remainingSignals)
	// Best case: every remaining signal scores 1.0.
	best := (sum + float64(remainingSignals)) / total
	// Worst case: every remaining signal scores 0.0.
	worst := sum / total
	if best < threshold {
		// Even with perfect remaining signals, we cannot clear the bar.
		return true
	}
	if worst >= threshold {
		// Even with zero remaining signals, we already cleared the bar.
		return true
	}
	return false
}

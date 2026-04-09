package research

import (
	"fmt"
	"strings"
	"sync"
)

// The prompts the research loop sends to the model used to live as
// hard-coded Go strings in this file. They now live as embedded .tmpl
// files under internal/research/prompts/, loaded through PromptStore.
// The exported functions in this file (PlanPrompt, DraftPrompt, etc.)
// are now thin wrappers that render the templates with the right
// variables — same callers, same return type, but the body is data.
//
// Why this matters: the planner template is the most-tuned surface in
// the loop. Forcing a Go recompile every time we wanted to test a
// copy change made iterative tuning impossible. Now `vim
// ~/.config/glitch/prompts/plan.tmpl` and the next research call
// picks it up. Brain-learned hints (next commit) feed the same
// template via the {{.Hints}} variable, so the system improves
// without anyone touching Go.

// defaultPromptStore is the package-level loader the existing
// exported helpers (PlanPrompt, DraftPrompt, ...) render through.
// Lazily constructed so importing the package never touches the
// filesystem — the loader probes ~/.config/glitch/prompts on
// construction. Tests that want a deterministic in-memory loader
// can override via SetDefaultPromptStore.
var (
	defaultPromptStoreOnce sync.Once
	defaultPromptStore     *PromptStore
)

func getDefaultPromptStore() *PromptStore {
	defaultPromptStoreOnce.Do(func() {
		defaultPromptStore = NewPromptStore("")
	})
	return defaultPromptStore
}

// SetDefaultPromptStore replaces the package-level loader. Used by
// the loop to inject a workspace-scoped store (so .glitch/prompts in
// the active workspace can override per-repo) and by tests that want
// to point at a stub. Concurrency-safe: the helpers below grab the
// store under the once-lock.
func SetDefaultPromptStore(s *PromptStore) {
	defaultPromptStoreOnce.Do(func() {})
	defaultPromptStore = s
}

// promptDataPlan is the template variable struct for plan.tmpl.
// Defined as a named type rather than an anonymous map so the
// template author can rely on stable field names — and so the brain
// hints reader (next commit) has a fixed slot to fill.
type promptDataPlan struct {
	Question    string
	Researchers []promptDataResearcher
	Hints       string // brain-learned routing hints; empty until wired
}

type promptDataResearcher struct {
	Name     string
	Describe string
}

// promptDataDraftish is shared by draft / critique / judge / verify
// templates. They all need the question, the bundle, and (for the
// non-draft slots) the current draft. Pulling them into one struct
// keeps the template variable surface tight.
type promptDataDraftish struct {
	Question    string
	Draft       string
	BundleItems []promptDataEvidence
}

type promptDataEvidence struct {
	Source string
	Title  string
	Body   string
	Refs   []string
}

// promptDataSelfConsistency is the variable struct for the
// self_consistency.tmpl template. It carries the original draft + the
// alternative drafts to compare against.
type promptDataSelfConsistency struct {
	Question     string
	Draft        string
	Alternatives []string
}

// PlanPrompt renders the plan.tmpl template with the supplied question
// and researcher menu. Backward-compatible wrapper around
// PlanPromptWithHints with an empty hint string — used by call sites
// that don't have a HintsProvider wired.
func PlanPrompt(question string, researchers []Researcher) string {
	return PlanPromptWithHints(question, researchers, "")
}

// PlanPromptWithHints renders the plan.tmpl template with the question,
// researcher menu, and an optional hint string sourced from the brain
// event log via HintsProvider. The hint lands in the template's
// {{.Hints}} slot and the default plan.tmpl wraps it in `{{if .Hints}}
// Brain hints (past calls for similar questions): ... {{end}}` so
// callers without a provider get the original behaviour automatically.
//
// This is the function the loop's plan stage calls. The hint is built
// per call (no caching) so a research call immediately benefits from
// events the previous research call wrote — the brain reads what it
// just wrote, no in-memory invalidation, no restart.
func PlanPromptWithHints(question string, researchers []Researcher, hints string) string {
	data := promptDataPlan{
		Question:    strings.TrimSpace(question),
		Researchers: make([]promptDataResearcher, 0, len(researchers)),
		Hints:       strings.TrimSpace(hints),
	}
	for _, r := range researchers {
		data.Researchers = append(data.Researchers, promptDataResearcher{
			Name:     r.Name(),
			Describe: strings.TrimSpace(r.Describe()),
		})
	}
	out, err := getDefaultPromptStore().Render(PromptNamePlan, data)
	if err != nil {
		// Defensive: an embedded template can't fail to render in
		// production (it's checked at test time), but if a user
		// override breaks parsing we still return SOMETHING the
		// loop can call the LLM with rather than crashing.
		return fmt.Sprintf("plan stage prompt failed to render (%v)\n\nquestion: %s", err, question)
	}
	return out
}

// DraftPrompt renders the draft.tmpl template with the question and
// the gathered evidence bundle.
func DraftPrompt(question string, bundle EvidenceBundle) string {
	data := promptDataDraftish{
		Question:    strings.TrimSpace(question),
		BundleItems: evidenceItemsForPrompt(bundle),
	}
	out, err := getDefaultPromptStore().Render(PromptNameDraft, data)
	if err != nil {
		return fmt.Sprintf("draft stage prompt failed to render (%v)\n\nquestion: %s", err, question)
	}
	return out
}

// evidenceItemsForPrompt is the small adapter that lifts a runtime
// EvidenceBundle into the template-friendly slice promptDataDraftish
// (and CritiquePrompt / JudgePrompt / VerifyPrompt) consume.
func evidenceItemsForPrompt(bundle EvidenceBundle) []promptDataEvidence {
	out := make([]promptDataEvidence, 0, bundle.Len())
	for _, ev := range bundle.Items {
		out = append(out, promptDataEvidence{
			Source: ev.Source,
			Title:  ev.Title,
			Body:   strings.TrimSpace(ev.Body),
			Refs:   append([]string(nil), ev.Refs...),
		})
	}
	return out
}

// ParsePlan extracts a JSON array of researcher names from a planner output
// string. It tolerates leading/trailing prose (small models occasionally
// preface their JSON despite the rule) by scanning for the first '[' and
// matching brackets. Names that are not strings or that fail validation
// against the registry are dropped by the loop, not by this parser.
//
// Three tolerance passes for the qwen2.5:7b output formats observed in
// the wild:
//
//   1. Strict JSON: `["git-log","github-prs"]` — the canonical form.
//   2. Escaped: `[\"git-log\"]` — the model tokenises its output
//      through a stringification layer. Strip one layer of \" → " and
//      retry.
//   3. Bare identifiers: `[git-log, github-prs]` — the model treats
//      researcher names as bareword tokens. Lex alphanumeric + hyphen
//      runs out of the bracketed region directly, no JSON involved.
//
// Each pass falls through to the next on failure; the original error
// is surfaced when all three fail so the caller can see what the model
// actually emitted.
func ParsePlan(raw string) ([]string, error) {
	if names, err := parsePlanRaw(raw); err == nil {
		return names, nil
	}
	// Pass 2: strip one layer of backslash escaping and retry.
	if unescaped := strings.ReplaceAll(raw, `\"`, `"`); unescaped != raw {
		if names, err := parsePlanRaw(unescaped); err == nil {
			return names, nil
		}
	}
	// Pass 3: bare-identifier lex. Find the first '[' and lex
	// alphanumeric/hyphen runs out of the substring up to the
	// matching ']' (or end-of-string). Tolerates spaces, commas,
	// and stray quotes between the identifiers.
	if names := parsePlanBareIdentifiers(raw); len(names) > 0 {
		return names, nil
	}
	return parsePlanRaw(raw) // surface the original error
}

// parsePlanBareIdentifiers is the third tolerance pass: rather than
// try to massage the model's output into valid JSON, lex
// alphanumeric+hyphen tokens out of the bracketed region directly.
// This rescues outputs like `[git-log, github-prs]` that some small
// models produce when they treat researcher names as barewords.
//
// Returns the empty slice when no identifiers are found, so the
// caller can fall through to the original-error path. Tokens are
// trimmed and de-duplicated to match parsePlanRaw's contract.
func parsePlanBareIdentifiers(raw string) []string {
	start := strings.Index(raw, "[")
	if start < 0 {
		return nil
	}
	end := strings.Index(raw[start:], "]")
	if end < 0 {
		end = len(raw)
	} else {
		end += start
	}
	body := raw[start+1 : end]

	var tokens []string
	var cur strings.Builder
	flush := func() {
		t := strings.TrimSpace(cur.String())
		cur.Reset()
		if t == "" {
			return
		}
		tokens = append(tokens, t)
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		isToken := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_'
		if isToken {
			cur.WriteByte(c)
			continue
		}
		flush()
	}
	flush()
	if len(tokens) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// parsePlanRaw is the bracket-matching JSON-array extractor; see ParsePlan
// for the public contract and the escape-stripping fallback.
func parsePlanRaw(raw string) ([]string, error) {
	start := strings.Index(raw, "[")
	if start < 0 {
		return nil, fmt.Errorf("research: planner output has no JSON array: %q", truncate(raw, 200))
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
				break
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("research: planner output has unbalanced JSON array: %q", truncate(raw, 200))
	}

	jsonText := raw[start:end]
	var names []string
	if err := jsonUnmarshalStrict(jsonText, &names); err != nil {
		return nil, fmt.Errorf("research: planner output is not a string array: %v: %q", err, truncate(jsonText, 200))
	}
	// Trim and de-duplicate while preserving order.
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

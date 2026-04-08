package research

import (
	"fmt"
	"strings"
)

// The prompts in this file are the contract between the loop and the local
// model. Each is written to address a specific failure mode observed in the
// pre-loop assistant — most importantly the "hallucinated PRs + suggest-don't-
// act" pattern captured in the project memory. Changing these prompts is a
// behavioural change to the loop and should be reviewed with the same care as
// changing the loop's Go code.

// PlanPrompt builds the planner prompt: given a question and the registry's
// (Name, Describe) menu, ask the model to emit a JSON array of researcher
// names whose evidence is needed.
//
// Rules baked in:
//   - The planner SHALL only emit names from the menu. The loop validates
//     this server-side; the rule in the prompt is belt-and-braces.
//   - "I would suggest using X" is not a valid plan. Picking X is.
//   - An empty plan is valid and means "no researcher fits" — the loop
//     short-circuits to a one-shot draft (which then must say "I don't have
//     enough evidence" because the bundle is empty).
func PlanPrompt(question string, researchers []Researcher) string {
	var b strings.Builder
	b.WriteString("You are the planning stage of a research loop. Your job is to pick which\n")
	b.WriteString("researchers should gather evidence for a user's question. You do NOT answer\n")
	b.WriteString("the question yourself. You do NOT explain how to answer it. You ONLY pick\n")
	b.WriteString("researcher names from the list below.\n\n")

	b.WriteString("Question:\n")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\n")

	b.WriteString("Available researchers (you may only pick names from this list):\n")
	if len(researchers) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, r := range researchers {
			fmt.Fprintf(&b, "- %s: %s\n", r.Name(), strings.TrimSpace(r.Describe()))
		}
	}
	b.WriteString("\n")

	b.WriteString("Rules:\n")
	b.WriteString("1. Output ONLY a JSON array of researcher names. No prose, no markdown, no explanation.\n")
	b.WriteString("2. Pick only names that appear verbatim in the list above.\n")
	b.WriteString("3. If a researcher's description matches the question's information needs, pick it.\n")
	b.WriteString("4. If no researcher fits the question, output [].\n")
	b.WriteString("5. NEVER invent a researcher name. NEVER suggest commands or tools the user should run.\n")
	b.WriteString("6. NEVER answer the question here. Picking is your only job.\n\n")

	b.WriteString("Output (JSON array only):\n")
	return b.String()
}

// DraftPrompt builds the prompt for the draft stage: given the question and
// the gathered evidence bundle, write an answer that is grounded ONLY in the
// bundle.
//
// Rules baked in:
//   - Every claim must be supported by an item in the evidence list. Specific
//     identifiers (PR numbers, file paths, commit SHAs, dates) MUST appear in
//     the evidence; otherwise the model must say "I don't have enough
//     evidence to answer that part."
//   - "You should run X" is forbidden. The model must report what the
//     evidence shows, not delegate to the user.
//   - If the bundle is empty, the model must say so explicitly. No summarising
//     from priors. No "based on my training data" answers.
func DraftPrompt(question string, bundle EvidenceBundle) string {
	var b strings.Builder
	b.WriteString("You are the drafting stage of a research loop. Your job is to answer the\n")
	b.WriteString("user's question using ONLY the evidence below. You may not invent facts.\n")
	b.WriteString("You may not use prior knowledge. You may not delegate to the user by saying\n")
	b.WriteString("'you should run X' — the evidence is what you have to work with.\n\n")

	b.WriteString("Question:\n")
	b.WriteString(strings.TrimSpace(question))
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

	b.WriteString("Rules:\n")
	b.WriteString("1. Cite specific identifiers (PR numbers, commit SHAs, file paths, dates, URLs)\n")
	b.WriteString("   ONLY when they appear verbatim in the evidence above. Never invent them.\n")
	b.WriteString("2. If — and ONLY if — the evidence contains nothing relevant to the question,\n")
	b.WriteString("   reply with exactly this single sentence and nothing else:\n")
	b.WriteString("   \"I don't have enough evidence to answer that.\"\n")
	b.WriteString("   When the evidence DOES support an answer (even partially), give that answer\n")
	b.WriteString("   and DO NOT append the no-evidence sentence. Never mix the two.\n")
	b.WriteString("3. Do not say \"you should run\" or \"I'd recommend running\" any command. Report\n")
	b.WriteString("   what the evidence shows, not what the user could do to find out themselves.\n")
	b.WriteString("4. Do not mention training data, prior conversations, or general knowledge.\n")
	b.WriteString("5. Be concise. Lead with the answer, not preamble.\n\n")

	b.WriteString("Answer:\n")
	return b.String()
}

// ParsePlan extracts a JSON array of researcher names from a planner output
// string. It tolerates leading/trailing prose (small models occasionally
// preface their JSON despite the rule) by scanning for the first '[' and
// matching brackets. Names that are not strings or that fail validation
// against the registry are dropped by the loop, not by this parser.
func ParsePlan(raw string) ([]string, error) {
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

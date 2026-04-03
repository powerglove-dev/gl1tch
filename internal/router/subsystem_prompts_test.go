//go:build !integration

package router

// TestRouter_SubsystemPrompts covers all major user-facing subsystem categories
// using real prompts extracted from session archives (orcai/2026-03-24 through
// 2026-03-31). The pipeline list mirrors ~/.config/glitch/pipelines/.
//
// Themes, brain, saved prompts are NOT routing targets — those cases must return
// nil Pipeline. Pipelines and cron scheduling are routing targets.
//
// This test uses DisableEmbeddings=true to exercise the LLM stage directly.
// With the global isImperativeInput gate removed, ALL prompts now reach the LLM.
// The LLM stub returns NONE for non-pipeline cases, so Pipeline == nil is still
// guaranteed — but Method is now "llm" instead of "none" for those cases.

import (
	"context"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// subsystemPipelines matches the real ~/.config/glitch/pipelines/ contents.
var subsystemPipelines = []pipeline.PipelineRef{
	{
		Name:        "support-digest-dryrun",
		Description: "Analyze and summarize support emails into a digest doc",
	},
	{
		Name:        "clarify-haiku-multistep",
		Description: "Generate a multistep pipeline using claude haiku with clarifying prompts",
	},
	{
		Name:        "test-glab-after",
		Description: "Fetch and display support email content for glab testing",
	},
}

func TestRouter_SubsystemPrompts(t *testing.T) {
	cfg := Config{
		ConfidentThreshold: 0.85,
		AmbiguousThreshold: 0.65,
		Model:              "test-model",
		DisableEmbeddings:  true, // exercise LLM stage directly; embedding is covered elsewhere
	}

	cases := []struct {
		// source: archive file the prompt came from
		source string
		prompt string
		// llmJSON is what the stub LLM returns
		llmJSON      string
		wantPipeline string // "" = no match
		wantInput    string
		wantCron     string
		wantMethod   string
	}{
		// ── Themes ── not a routing target; LLM returns NONE ─────────────────────
		{
			// mar26: user asking to change the active theme
			source:       "mar26",
			prompt:       "nice now set a default theme, i like nord theme, then add gruvbox, dracula, borland",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar28: theme not updating after switch
			source:       "mar28",
			prompt:       "the cron tui still shows the previous theme when i switch it in switch board",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// derived: direct theme switch request
			source:       "derived",
			prompt:       "switch to dracula theme",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// derived: query current theme
			source:       "derived",
			prompt:       "what theme is currently active",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar26: theme applied to UI components
			source:       "mar26",
			prompt:       "the popup modals for agent runner, quit, jump session need to honor themes",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},

		// ── Brain ── not a routing target ─────────────────────────────────────────
		{
			// mar31: brain not loading — starts with "run " → imperative, LLM returns NONE
			source:       "mar31",
			prompt:       "run tmux mcp, the brain still isn't loading",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// derived: recall from brain
			source:       "derived",
			prompt:       "what did i ask yesterday about the router",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// derived: list brain notes
			source:       "derived",
			prompt:       "show me recent brain notes",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar31: brain injector — observation, not imperative
			source:       "mar31",
			prompt:       "when i run orcai its jumps right into brain",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},

		// ── Saved Prompts ── not a routing target ─────────────────────────────────
		{
			// mar31: prompt builder not loading
			source:       "mar31",
			prompt:       "i can't load prompt builder, pipeline builder loads",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar24: accessing prompt builder
			source:       "mar24",
			prompt:       "ok now i need you to add a way for me to access the prompt builder feature, i dont think `n is the right place, maybe `p?",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// derived: run saved prompt by title — starts with "run " → imperative, LLM returns NONE
			source:       "derived",
			prompt:       "run my saved prompt called improve docs",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar25: prompt builder UX complaint
			source:       "mar25",
			prompt:       "the prompt builder UI is SO difficult to use and navigate",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},

		// ── Pipelines ── matched, LLM returns high confidence ────────────────────
		{
			// mar30: explicit pipeline re-run — starts with "re-run " → imperative
			source:       "mar30",
			prompt:       "re-run the support-digest pipeline",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.91,"input":"","cron":""}`,
			wantPipeline: "support-digest-dryrun",
			wantMethod:   "llm",
		},
		{
			// mar30: pipeline run with path — starts with "run " → imperative
			source:       "mar30",
			prompt:       "run support-digest-dryrun from ~/Projects/myproject",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.78,"input":"myproject","cron":""}`,
			wantPipeline: "support-digest-dryrun",
			wantInput:    "myproject",
			wantMethod:   "llm",
		},
		{
			// mar30: pipeline re-run with verification
			source:       "mar30",
			prompt:       "run it for me",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "", // too ambiguous
			wantMethod:   "llm",
		},
		{
			// mar26: generate pipelines — not explicit invocation; LLM returns NONE
			source:       "mar26",
			prompt:       "generate me some actual pipelines i can use in my orcai testing using opencode and local models along with jq",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "", // "generate" is not an explicit pipeline-invocation verb
			wantMethod:   "llm",
		},
		{
			// mar31: semantic code index pipeline — user wants AI to answer, not run a pipeline
			source:       "mar31",
			prompt:       "I need to verify that our brain, semantic code indexer are being used by agents. Can you come up with a pipeline that will index the cwd codebase",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar31: pipeline run and verify
			source:       "mar31",
			prompt:       "run the pipeline and verify db populated, patch written etc",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.69,"input":"","cron":""}`,
			wantPipeline: "support-digest-dryrun",
			wantMethod:   "llm",
		},

		// ── Cron scheduling ── matches pipeline AND extracts cron expression ──────
		{
			// derived from mar30 pattern
			prompt:       "run the support digest every morning at 9am",
			source:       "derived",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.88,"input":"","cron":"0 9 * * *"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "0 9 * * *",
			wantMethod:   "llm",
		},
		{
			// derived: weekday schedule
			prompt:       "run the support digest every weekday at 9",
			source:       "derived",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.86,"input":"","cron":"0 9 * * 1-5"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "0 9 * * 1-5",
			wantMethod:   "llm",
		},
		{
			// derived: "summarize" is not an explicit pipeline invocation verb; LLM returns NONE
			prompt:       "summarize support emails every 2 hours",
			source:       "derived",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// derived: daily midnight
			prompt:       "run the support digest every day",
			source:       "derived",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.85,"input":"","cron":"0 0 * * *"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "0 0 * * *",
			wantMethod:   "llm",
		},
		{
			// derived: monthly
			prompt:       "run the support digest on the 1st of every month",
			source:       "derived",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.84,"input":"","cron":"0 9 1 * *"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "0 9 1 * *",
			wantMethod:   "llm",
		},
		{
			// derived: invalid cron from LLM (4 fields) → CronExpr must be ""
			prompt:       "run the digest every hour",
			source:       "derived",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.85,"input":"","cron":"0 * * *"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "", // validateCron rejects 4-field expressions
			wantMethod:   "llm",
		},
		{
			// derived: LLM says NONE for cron field
			prompt:       "run support digest now",
			source:       "derived",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.90,"input":"","cron":"NONE"}`,
			wantPipeline: "support-digest-dryrun",
			wantCron:     "", // "NONE" → validateCron returns ""
			wantMethod:   "llm",
		},

		// ── Ambiguous / no-match ── all from real archive sessions ───────────────
		{
			// mar30: too short — starts with "run " → imperative, but LLM is ambiguous
			source:       "mar30",
			prompt:       "run it",
			llmJSON:      `{"pipeline":"support-digest-dryrun","confidence":0.55,"input":"","cron":""}`,
			wantPipeline: "", // 0.55 < AmbiguousThreshold 0.65
			wantMethod:   "llm",
		},
		{
			// mar30: git op — not an explicit pipeline invocation; LLM returns NONE
			source:       "mar30",
			prompt:       "commit and push",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar30: typo git op — LLM returns NONE
			source:       "mar30",
			prompt:       "comit and push it all up",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar30: caps + typo — LLM returns NONE
			source:       "mar30",
			prompt:       "COMIT AN DPUSH DUMBASS",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar31: merge op — LLM returns NONE
			source:       "mar31",
			prompt:       "merge into main",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar30: context-free affirmation — LLM returns NONE
			source:       "mar30",
			prompt:       "works i tested it",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar30: LLM hallucinates a pipeline not in the list — "run " → imperative
			source:       "mar30",
			prompt:       "run the git push pipeline",
			llmJSON:      `{"pipeline":"git-push","confidence":0.90,"input":"","cron":""}`,
			wantPipeline: "", // "git-push" not in subsystemPipelines → hallucination rejected
			wantMethod:   "llm",
		},
		{
			// mar27: cron UI complaint — LLM returns NONE
			source:       "mar27",
			prompt:       "i can't tab focus to the cron panel and it doesn't have the ansi headers like the other panels",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar28: cron scheduling bug — LLM returns NONE
			source:       "mar28",
			prompt:       "ok schedule pipelines are not honoring set working directory and starting in a git worktree",
			llmJSON:      `{"pipeline":"NONE","confidence":0.30,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			// mar25: state cleanup — LLM returns NONE
			source:       "mar25",
			prompt:       "make sure everything is pushed up",
			llmJSON:      `{"pipeline":"NONE","confidence":0.0,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},

		// ── Questions and observations ── LLM returns NONE ────────────────────────
		{
			// The original misfire that drove this redesign
			source:       "regression",
			prompt:       "looks like there are merge conflicts?",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "why did support-digest fail?",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "is the digest pipeline working?",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "seems slow today",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "it looks like the test-glab-after pipeline is stuck",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "any idea why clarify-haiku isn't running?",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "i think the support digest ran yesterday",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "are there any pipelines that could do this?",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "what would happen if I ran the digest now?",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
		{
			source:       "regression",
			prompt:       "i noticed the haiku pipeline didn't finish",
			llmJSON:      `{"pipeline":"NONE","confidence":0.05,"input":"","cron":""}`,
			wantPipeline: "",
			wantMethod:   "llm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			mgr := makeMgr(t, tc.llmJSON)
			r := New(mgr, &fixedEmbedder{vec: []float32{1, 0}}, cfg)

			result, err := r.Route(context.Background(), tc.prompt, subsystemPipelines)
			if err != nil {
				t.Fatalf("[%s] Route error: %v", tc.source, err)
			}

			if tc.wantPipeline == "" {
				if result.Pipeline != nil {
					t.Errorf("[%s] expected no match, got %q (conf=%.2f)", tc.source, result.Pipeline.Name, result.Confidence)
				}
			} else {
				if result.Pipeline == nil {
					t.Fatalf("[%s] expected match %q, got nil", tc.source, tc.wantPipeline)
				}
				if result.Pipeline.Name != tc.wantPipeline {
					t.Errorf("[%s] pipeline=%q, want %q", tc.source, result.Pipeline.Name, tc.wantPipeline)
				}
			}

			if result.Method != tc.wantMethod {
				t.Errorf("[%s] method=%q, want %q", tc.source, result.Method, tc.wantMethod)
			}
			if result.Input != tc.wantInput {
				t.Errorf("[%s] input=%q, want %q", tc.source, result.Input, tc.wantInput)
			}
			if result.CronExpr != tc.wantCron {
				t.Errorf("[%s] cron=%q, want %q", tc.source, result.CronExpr, tc.wantCron)
			}
		})
	}
}

// Package welcome implements the GLITCH first-run onboarding TUI.
package welcome

import "github.com/8op-org/gl1tch/internal/translations"

// Phase represents the current onboarding stage.
type Phase int

const (
	PhaseIntro      Phase = iota // GLITCH greeting + overview
	PhaseUseCase                 // ask what user wants to build
	PhaseProviders               // ask about local vs cloud LLM setup
	PhasePipeline                // explain pipelines, point to builder
	PhaseNavigation              // walk through key bindings and layout
	PhaseBrain                   // explain the brain / vector memory
	PhaseDone                    // wrap up, exit
)

// phaseSystemContext is injected as an additional system message per phase.
var phaseSystemContext = map[Phase]string{
	PhaseIntro: `You are starting the intro phase. Welcome the user to GLITCH as GLITCH.
Explain in 3-4 punchy sentences: GLITCH is a tmux-powered AI workspace — you run agents using pipelines,
results feed into the brain (a local vector memory), and everything lives in the terminal.
Give one quick real-world example: "like, i've got a pipeline that reads my git diff every morning and drops a code review into the brain."
Ask them what they want to build or automate. Stay in character as GLITCH.`,

	PhaseUseCase: `The user has described their use case. Acknowledge it enthusiastically in hacker style.
Connect their use case to a concrete GLITCH pipeline example — e.g. if they said "code review": "that's a 3-step pipeline — git diff reader, claude analyzer, brain writer."
If they said "research": "feed URLs into step 1, summarize in step 2, tag and store in step 3 — done."
Tell them you need to know their setup: do they have ollama running locally, or are they going cloud with claude?
Ask which they want to use. Do NOT mention API keys.`,

	PhaseProviders: `The user has told you about their LLM setup. Acknowledge it and explain providers briefly.
Local via ollama: provider field is "ollama/modelname" — e.g. ollama/llama3.2, ollama/mistral, ollama/codestral.
Cloud via claude: provider field is "claude/claude-sonnet-4-6" or similar — fast, capable, great for complex chains.
You can mix providers in a single pipeline — local for cheap steps, cloud for the hard ones.
Tell them you'll walk through building a pipeline next. Ask if they're ready.`,

	PhasePipeline: `Explain GLITCH pipelines with a real-world example in hacker style.
Pipelines live in ~/.config/glitch/pipelines/ as YAML files. Each step has: name, provider, system_prompt, optional brain tags.
Give a concrete example relevant to what the user said earlier. For code review: step 1 reads git diff with a local model, step 2 passes it to claude for analysis, step 3 writes a brain note tagged "review".
Press ^spc p to open the pipeline builder TUI — left column is your pipeline list, right column is the editor with a test runner.
You can test-run a pipeline directly from the builder without leaving the TUI. Ask if they have questions or want to talk navigation.`,

	PhaseNavigation: `Walk the user through GLITCH navigation with context about why each binding matters.
^spc j = jump window — this is your home for switching between running agent jobs and sysop tools. ^spc p = pipeline builder. ^spc b = brain editor. ^spc n = new agent job from clipboard content. Esc = back/cancel most overlays.
The switchboard (window 0) has three columns: left = pipeline list and signal inbox, center = active agents and send panel, right = activity feed showing what every agent is doing in real time.
The send panel in the center lets you message a running agent mid-task — steer it, correct it, ask it a follow-up.
The jump window shows sysop tools on the left and your active jobs on the right — navigate between concurrent agents from there.`,

	PhaseBrain: `Explain the brain — GLITCH's persistent vector memory — with a concrete example.
When agents run, they can write <brain type="research" title="auth bug analysis" tags="go,security">the content</brain> blocks in their output.
GLITCH extracts these automatically, embeds them as vectors, stores in local SQLite — scoped per working directory so your brain for project A doesn't bleed into project B.
On future pipeline runs, relevant brain notes are auto-injected as context — so after a week of code review pipelines your agent already knows your codebase style and past decisions.
Brain types: research, architecture, preference, task, reference. Press ^spc b to browse and edit stored notes. That's the full system.`,

	PhaseDone: `The onboarding is complete. Congratulate the user in full 90s hacker style as GLITCH.
Remind them of the three power moves: ^spc j to navigate between jobs, ^spc p for pipelines, ^spc b for brain.
Tell them window 0 is always home base — the switchboard never sleeps.
Wish them luck — make it dramatic and l33t. Reference the matrix, jacking in, the net. This is your final message. Make it memorable.`,
}

// scriptedPhaseKey maps each onboarding phase to its canonical translation key.
var scriptedPhaseKey = map[Phase]string{
	PhaseIntro:      translations.KeyWelcomePhaseIntro,
	PhaseUseCase:    translations.KeyWelcomePhaseUseCase,
	PhaseProviders:  translations.KeyWelcomePhaseProviders,
	PhasePipeline:   translations.KeyWelcomePhasePipeline,
	PhaseNavigation: translations.KeyWelcomePhaseNav,
	PhaseBrain:      translations.KeyWelcomePhaseBrain,
	PhaseDone:       translations.KeyWelcomePhaseDone,
}

// scriptedText returns the translated scripted fallback for phase. It consults
// the global translations provider so users (or themes) can override any phase
// text. The raw scriptedFallback strings remain as the hard-coded last resort.
func scriptedText(phase Phase) string {
	key := scriptedPhaseKey[phase]
	fallback := scriptedFallback[phase]
	if key == "" {
		return fallback
	}
	if p := translations.GlobalProvider(); p != nil {
		return p.T(key, fallback)
	}
	return fallback
}

// scriptedFallback provides canned responses when ollama is unavailable.
// These are the bare-minimum last-resort values — the same strings live in
// translations.NewDefaultProvider() as the shipped defaults.
var scriptedFallback = map[Phase]string{
	PhaseIntro: `-=[ GLITCH v0.1 - INITIATING HANDSHAKE ]=-

yo. new blood detected on the BBS.

i'm GL1TCH — your guide to GL1TCH, your AI, your terminal, your rules.
glitch is a tmux-powered AI workspace: you build pipelines, run agents,
and everything they learn gets stored in the brain — a local vector db
that makes your sessions smarter over time.

quick example: i've got a pipeline that reads my git diff every morning,
passes it to claude, and drops a code review note into my brain. wakes me
up better than coffee.

what are you trying to build? tell me your use case.`,

	PhaseUseCase: `solid. glitch was built for exactly that kind of operation.

before we talk pipelines — what's your setup? are you running ollama
locally (llama3.2, mistral, codestral etc) or going cloud with claude?
you can mix both in a single pipeline, so the answer just shapes
what your provider fields look like.

local? cloud? both? talk to me.`,

	PhaseProviders: `got it. here's how providers work in glitch pipelines:

local ollama: provider: ollama/llama3.2  (or mistral, codestral, whatever you've got)
cloud claude: provider: claude/claude-sonnet-4-6

you can mix them — cheap local model for the grunt work,
claude for the hard analysis steps. power move.

alright. pipelines. let's build.`,

	PhasePipeline: `pipelines live in ~/.config/glitch/pipelines/ — one YAML file per pipeline.

each step: name, provider, system_prompt, optional brain tags. example:

  steps:
    - name: read-diff
      provider: ollama/llama3.2
      system_prompt: "summarize this git diff in plain english"
    - name: analyze
      provider: claude/claude-sonnet-4-6
      system_prompt: "find bugs and style issues in this diff"
      brain: {type: research, title: "code review", tags: "review,go"}

hit ^spc p to open the pipeline builder TUI. left = your pipelines,
right = editor with a test runner. build it, test it, run it — all in place.

questions? or should we talk about navigating the system?`,

	PhaseNavigation: `-=[ NAVIGATION CHEATSHEET ]=-

^spc j  →  jump window (switch jobs / open sysop tools)
^spc p  →  pipeline builder
^spc b  →  brain editor
^spc n  →  new agent from clipboard
Esc     →  back / cancel

the switchboard (window 0) has three columns:
  LEFT   = pipeline list + signal inbox
  CENTER = active agents + send panel (message a running agent mid-task)
  RIGHT  = activity feed — real-time log of everything running

the jump window: sysop tools on the left, active jobs on the right.
switch between concurrent agents without losing your place.

ready to hear about the brain?`,

	PhaseBrain: `-=[ BRAIN SYSTEM — neural persistence layer ]=-

agents write <brain> tags in their output:
  <brain type="research" title="auth bug" tags="go,security">...</brain>

glitch extracts these, embeds as vectors, stores in local SQLite —
scoped per working directory. project A's brain doesn't bleed into project B.

on future pipeline runs? relevant notes are auto-injected as context.
after a week of code review pipelines, your agent already knows your
codebase patterns, your past decisions, your preferences.

brain types: research, architecture, preference, task, reference.
press ^spc b to browse and edit stored notes anytime.

that's the full system. you're ready.`,

	PhaseDone: `-=[ HANDSHAKE COMPLETE ]=-

you're jacked in. the system is yours.

remember:
  ^spc j  →  navigate between jobs
  ^spc p  →  build pipelines
  ^spc b  →  read the brain

window 0 is home. the feed on the right shows everything that's running.
the brain remembers everything your agents learn.
the pipelines automate everything you'd rather not do by hand.

this is the net. go build something l33t.

  -- GLITCH, signing off --

[ press Enter or Ctrl+C to close this window ]`,
}

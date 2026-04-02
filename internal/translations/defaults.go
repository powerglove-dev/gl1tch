package translations

// NewDefaultProvider returns a MapProvider pre-loaded with the GL1TCH default
// UI strings. These are the shipped personality baseline — zer0c00l voice,
// dry wit, hacker confidence. Users and themes can override any key; this
// layer sits at the bottom of the ChainProvider stack.
func NewDefaultProvider() Provider {
	return NewMapProvider(defaultStrings)
}

// defaultStrings holds the canonical default value for every translation key.
// Voice: confident, dry, precise. No hype. Treats the reader as a peer.
// Inspired by Zero Cool (Hackers, 1995) and the Sneakers (1992) crew.
var defaultStrings = map[string]string{
	// ── Panel titles ─────────────────────────────────────────────────────────
	KeyPipelinesTitle:    "PIPELINES",
	KeyAgentRunnerTitle:  "AGENT RUNNER",
	KeySignalBoardTitle:  "SIGNAL BOARD",
	KeyActivityFeedTitle: "ACTIVITY FEED",
	KeyInboxTitle:        "INBOX",
	KeyCronTitle:         "CRON JOBS",

	// ── Header / modal titles ─────────────────────────────────────────────────
	KeyDeckHeader: "GL1TCH",
	KeyQuitModalTitle:    "BAIL OUT",
	KeyHelpModalTitle:    "GETTING STARTED",
	KeyThemePickerTitle:  "SELECT THEME",

	// ── Quit modal ────────────────────────────────────────────────────────────
	KeyQuitConfirmMessage: "you sure? the grid will still be here.",

	// ── Help modal ────────────────────────────────────────────────────────────
	KeyHelpChordNote:        "Chord prefix: ^spc  (ctrl+space, then the key below)",
	KeyHelpSectionSystem:    "HELP & SYSTEM",
	KeyHelpSectionWorkspace: "WORKSPACE",
	KeyHelpSectionWindows:   "WINDOWS & PANES",
	KeyHelpSectionNav:       "NAVIGATION",
	KeyHelpSectionPanels:    "PANELS",

	// Help binding descriptions
	KeyHelpBindHelp:    "this help",
	KeyHelpBindQuit:    "quit GLITCH",
	KeyHelpBindDetach:  "detach  (session stays alive)",
	KeyHelpBindReload:  "reload  (hot-swap binary)",
	KeyHelpBindThemes:  "theme picker",
	KeyHelpBindJump:    "jump to any window",
	KeyHelpBindNewWin:  "new window",
	KeyHelpBindPrevWin: "previous / next window",
	KeyHelpBindSplitR:  "split pane right / down",
	KeyHelpBindNavPane: "navigate panes",
	KeyHelpBindKill:    "kill pane / window",
	KeyHelpBindTabNav:  "navigate panels & list items",
	KeyHelpBindEnter:   "open / confirm / run selected",
	KeyHelpBindEsc:     "back / close overlay",

	// Help panel descriptions
	KeyHelpPanelPipelines:    "run and monitor named pipelines",
	KeyHelpPanelAgentRunner:  "launch and wrangle AI agents",
	KeyHelpPanelSignalBoard:  "inter-agent message bus",
	KeyHelpPanelActivityFeed: "live events, run history, logs",
	KeyHelpPanelCron:         "scheduled pipelines and agent jobs",

	// ── Theme picker ──────────────────────────────────────────────────────────
	KeyThemePickerDarkTab:  "Dark",
	KeyThemePickerLightTab: "Light",

	// ── Rerun modal ───────────────────────────────────────────────────────────
	KeyRerunContextLabel: "ADDITIONAL CONTEXT",
	KeyRerunCwdLabel:     "WORKING DIRECTORY",

	// ── Welcome onboarding phases ─────────────────────────────────────────────
	// These are the scripted fallbacks used when ollama is not available.
	// The LLM-driven phaseSystemContext prompts are separate and not translated.
	KeyWelcomePhaseIntro: `-=[ GLITCH v0.1 - INITIATING HANDSHAKE ]=-

yo. new blood detected on the BBS.

i'm GL1TCH — your guide to GL1TCH, your AI, your terminal, your rules.
glitch is a tmux-powered AI workspace: you build pipelines, run agents,
and everything they learn gets stored in the brain — a local vector db
that makes your sessions smarter over time.

quick example: i've got a pipeline that reads my git diff every morning,
passes it to claude, and drops a code review note into my brain. wakes me
up better than coffee.

what are you trying to build? tell me your use case.`,

	KeyWelcomePhaseUseCase: `solid. glitch was built for exactly that kind of operation.

before we talk pipelines — what's your setup? are you running ollama
locally (llama3.2, mistral, codestral etc) or going cloud with claude?
you can mix both in a single pipeline, so the answer just shapes
what your provider fields look like.

local? cloud? both? talk to me.`,

	KeyWelcomePhaseProviders: `got it. here's how providers work in glitch pipelines:

local ollama: provider: ollama/llama3.2  (or mistral, codestral, whatever you've got)
cloud claude: provider: claude/claude-sonnet-4-6

you can mix them — cheap local model for the grunt work,
claude for the hard analysis steps. power move.

alright. pipelines. let's build.`,

	KeyWelcomePhasePipeline: `pipelines live in ~/.config/glitch/pipelines/ — one YAML file per pipeline.

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

	KeyWelcomePhaseNav: `-=[ NAVIGATION CHEATSHEET ]=-

^spc j  →  jump window (switch jobs / open sysop tools)
^spc p  →  pipeline builder
^spc b  →  brain editor
^spc n  →  new agent from clipboard
Esc     →  back / cancel

the deck (window 0) has three columns:
  LEFT   = pipeline list + signal inbox
  CENTER = active agents + send panel (message a running agent mid-task)
  RIGHT  = activity feed — real-time log of everything running

the jump window: sysop tools on the left, active jobs on the right.
switch between concurrent agents without losing your place.

ready to hear about the brain?`,

	KeyWelcomePhaseBrain: `-=[ BRAIN SYSTEM — neural persistence layer ]=-

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

	KeyWelcomePhaseDone: `-=[ HANDSHAKE COMPLETE ]=-

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

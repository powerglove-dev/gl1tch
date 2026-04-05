---
name: gl1tch-site
description: Manage the gl1tch website — docs, frontend components, pages, and screenshots. Use whenever the user wants to update or write documentation, change the homepage or site layout, add or modify Astro components, generate or refresh TUI screenshots for the site, or run any site-related workflow. Triggers on phrases like "update docs for X", "improve the docs", "document this", "update the homepage", "add a component", "take a screenshot of the TUI", "refresh screenshots", "update the site", "write docs for this feature". Trigger immediately without asking for confirmation.
---

The gl1tch site is a static Astro v6 build. All source lives in `site/src/`. Changes reach production after a PR merges to main — `gh-pages.yml` rebuilds and deploys to 8op.org automatically.

## What this skill manages

| Surface | Source | How to change |
|---------|--------|--------------|
| Docs pages | `site/src/content/pipelines/*.md` | docs-improve pipeline or direct edit |
| Astro components | `site/src/components/*.astro` | edit + rebuild |
| Pages | `site/src/pages/` | edit + rebuild |
| Screenshots | `site/public/screenshots/<group>/<name>.png` | scenario files + tuishot |

## The two goals of all gl1tch content

Every page, component, and screenshot exists for one of these reasons:

1. **Get running fast** — a new user should run their first pipeline in under 5 minutes
2. **Make it yours** — experienced users customizing providers, pipelines, themes, automations

Internal implementation details (tmux, BubbleTea, OTel, Go packages, SQLite) never appear in user-facing content. Lead with what the user *does*, not how it's built. Use "your" framing throughout — your assistant, your pipelines, your brain. Put a working example before any explanation.

## The editorial rule: gl1tch builds gl1tch

The most important message across all docs and site copy: **you use gl1tch to create things, not a text editor.**

When documenting pipelines, prompts, workflows, or cron jobs, the primary path always shows gl1tch doing the creation:

```
# Right — gl1tch is the author
ask glitch: "create a pipeline that summarizes my git log every morning"

# Wrong — user is hand-editing YAML
Open ~/.config/glitch/pipelines/summary.pipeline.yaml and add...
```

YAML is the underlying format and power users will reach for it — that's fine and worth a brief mention. But it's always secondary. The headline example, the quickstart step, the feature card copy: all of it shows gl1tch generating the artifact, not the user writing it by hand.

Think of it like this: a cookware brand's recipes don't start with "first, source your own copper." They show you making dinner. gl1tch docs show you building automations, not configuring files.

**Concretely, when docs cover any of these topics:**
- Pipelines → show `/pipeline create` or asking the console to generate one first; YAML reference comes after
- Prompts → show saving a prompt from a conversation; raw prompt file format is a footnote
- Workflows → show chaining via the console or a pipeline step; manual orchestration is advanced
- Cron jobs → show `/cron add` first; cron YAML fields are reference material, not the intro
- Plugins → show `apm install <name>` first; building a plugin from scratch is an advanced page

If a reviewed doc leads with file editing before showing what gl1tch can generate or do, flag it and re-run the pipeline with an explicit note to reframe the intro.

## Every example must be verifiable

No example in the docs is allowed to be illustrative or hypothetical. If it shows a gl1tch command or a conversation with the assistant, it must have been run and confirmed to produce real output.

Before any doc ships, verify each example by running it through gl1tch:

```bash
glitch ask "<the example prompt from the doc>"
```

Capture the actual output. The example in the doc should reflect what gl1tch actually says or does — not a cleaned-up invention of what it should say. If the output is long, trim it for readability, but never fabricate or paraphrase it into something the tool didn't produce.

**What this prevents:** a doc that says "ask glitch: 'create a daily summary pipeline'" and then shows a perfect-looking YAML that gl1tch never actually generated. That breaks user trust the moment they try it and get something different.

**Practical flow for the docs-improve pipeline:** after the `write_doc` step produces a draft, the `verify` step must run any inline examples through `glitch ask` and either confirm output matches or flag the example for correction before the PR opens. A doc with unverified examples must not auto-merge.

**A failing example is a bug, not a doc problem.** If `glitch ask "<example prompt>"` doesn't produce the output the doc claims, stop. Do not patch the doc to match whatever gl1tch actually said. The feature is broken or the command doesn't exist yet — that is the thing to fix. Open a bug report or fix the underlying behavior first, then re-run docs-improve once gl1tch can actually do what the doc says. Shipping a doc that describes behavior gl1tch can't reproduce is worse than no doc at all.

---

## Mode 1: Docs

Run the docs-improve pipeline with the extracted focus:

```bash
cd /Users/stokes/Projects/gl1tch && glitch pipeline run docs-improve --input "<focus>"
```

If no specific focus was given, omit `--input` and let the pipeline pick the highest-priority gap.

**Focus phrasing** — describe what the user does with the feature, not its internal name:
- "cron" → `"scheduling pipelines to run automatically"`
- "brain" → `"gl1tch remembering context across sessions"`
- "console" → `"the interactive AI assistant"`

**Two outcomes:**
- **`AUTO_MERGED:`** — pipeline merged the PR and CI/CD will rebuild the site automatically. Done.
- **`MISSING_TESTS:`** — PR is open but has test gaps. Write missing tmux integration tests before the PR can merge.

**Writing missing tests** (when `MISSING_TESTS:` appears):

For each listed command, add a test to `internal/console/tui_integration_test.go`:

```go
func TestTmux_Cmd_Foo(t *testing.T) {
    session, _, cleanup := setupTUISession(t, "foo", nil)
    defer cleanup()

    sendSlashCmd(t, session, "/foo")

    ok := waitFor(3*time.Second, func() bool {
        c := tmuxCapture(t, session)
        return strings.Contains(c, "<real expected string from source>")
    })
    if !ok {
        t.Errorf("/foo did not produce expected output:\n%s", tmuxCapture(t, session))
    }
    if !isTUIAlive(t, session) {
        t.Errorf("TUI died after /foo")
    }
}
```

Read the actual handler in `internal/console/` to find the real response string — never guess. Then commit tests to the pipeline's PR branch:

```bash
BRANCH=$(git branch --list "docs/improve-*" --sort=-creatordate | head -1 | tr -d ' *')
git checkout "$BRANCH"
git add internal/console/tui_integration_test.go
git commit -m "test(console): add tmux integration tests for documented TUI actions"
git push
git checkout main
```

---

## Mode 2: Frontend components and pages

The site uses Astro v6. Components live in `site/src/components/`, pages in `site/src/pages/`.

**Existing components:**
- `MacTerminal.astro` — macOS-style terminal window on the homepage
- `FeatureCard.astro` — individual feature highlight cards
- `AnsiLogo.astro` — ANSI art logo rendering
- `TuiBox.astro` — box-drawing terminal chrome
- `SysinfoBox.astro` — system info display panel
- `ProjectStats.astro` — live stats badges
- `GlitchLogo.astro` — animated logo wrapper
- `screens/DocsScreen.astro` — inline `/docs` panel (embedded in the terminal UI)

**When creating or modifying components:**

1. Read the existing component first to understand its props, slots, and style patterns
2. Use only the Dracula palette — check `site/public/css/` for the defined variables. Never introduce new color values
3. Use monospace fonts throughout — this is a terminal-aesthetic site, not a design portfolio
4. Match the existing box-drawing and ASCII aesthetic; no rounded cards, drop shadows, or gradient backgrounds unless already present on the page
5. Verify the build passes before opening a PR:
   ```bash
   cd /Users/stokes/Projects/gl1tch/site && npm run build
   ```

After editing, open a PR targeting `main` with your `site/**` changes. CI/CD handles the rest.

---

## Mode 3: Screenshots

Screenshots are PNG assets committed to `site/public/screenshots/<group>/`. They are captured from a live gl1tch TUI running in iTerm2 using `screencapture` — not tmux, not tuishot.

**All screenshots must be the same size.** The standard window geometry is **120 columns × 50 rows**. Always set this before capturing.

### Scenario files

Scenario files live at `site/src/scenarios/<name>.yaml`. Each one describes the TUI state to reach before taking a screenshot, making every screenshot reproducible and refreshable.

```yaml
name: console-welcome
description: gl1tch main console ready state — shown on the homepage
group: console
steps:
  - launch: glitch
  - wait: 3
  - screenshot: welcome
```

```yaml
name: session-tabs
description: Footer tab bar showing gl1tch tab and an active claude session
group: console
steps:
  - launch: glitch
  - wait: 3
  - send: "/session new claude"
  - wait: 2
  - screenshot: session_tabs
```

**Scenario step reference:**

| step | what it does |
|------|-------------|
| `launch: glitch` | Opens a new iTerm2 window at 120×50, runs `glitch`, brings iTerm2 to front |
| `send: "<text>"` | `osascript` write text → Enter in the current iTerm2 session |
| `wait: N` | `sleep N` — pause N seconds for TUI to render |
| `screenshot: <name>` | Captures the iTerm2 window to `site/public/screenshots/<group>/<name>.png` |
| `quit` | Sends `/quit` Enter, sleeps 1s, closes the iTerm2 window |

### Executing a scenario

Read the YAML and execute each step using osascript + screencapture. The group comes from the scenario frontmatter.

**Step: launch**
```bash
osascript <<'ASEOF'
tell application "iTerm2"
  activate
  set newWindow to (create window with default profile)
  tell newWindow
    tell current session
      set columns to 120
      set rows to 50
      write text "glitch"
    end tell
  end tell
end tell
ASEOF
```

**Step: send**
```bash
osascript -e 'tell application "iTerm2" to tell current window to tell current session to write text "<text>"'
```

**Step: wait**
```bash
sleep <N>
```

**Step: screenshot** (group=console, name=welcome)
```bash
REPO="/Users/stokes/Projects/gl1tch"
mkdir -p "$REPO/site/public/screenshots/console"
WIN_ID=$(osascript -e 'tell application "iTerm2" to get id of front window')
screencapture -x -l "$WIN_ID" "$REPO/site/public/screenshots/console/welcome.png"
```

**Step: quit**
```bash
osascript -e 'tell application "iTerm2" to tell current window to tell current session to write text "/quit"'
sleep 1
osascript -e 'tell application "iTerm2" to tell front window to close'
```

### Refreshing all screenshots

When the user asks to "refresh screenshots" or "update screenshots for the site", execute each scenario file sequentially:

```bash
for f in /Users/stokes/Projects/gl1tch/site/src/scenarios/*.yaml; do
  # read and execute scenario $f
done
```

After all scenarios complete, commit the changed PNGs:

```bash
git add site/public/screenshots/
git commit -m "chore(site): refresh TUI screenshots"
```

Then open a PR or push directly if the user prefers.

### Screenshot groups

| group | when to use |
|-------|-------------|
| `console` | Main switchboard / assistant UI |
| `pipelines` | Pipeline run output |
| `cron` | Scheduled job output |
| `plugins` | Plugin install/list/remove |
| `general` | Anything that doesn't fit above |

---

## Local build and preview

```bash
# Full build
cd /Users/stokes/Projects/gl1tch/site && npm run build

# Dev server with hot reload
task site:dev

# Regenerate response database (homepage terminal)
task site:glitch-db
```

---

## PR workflow

All site changes follow the same path:

1. Commit changes to a feature branch
2. PR targeting `main`
3. After merge, `gh-pages.yml` rebuilds and deploys to 8op.org automatically

For docs, the pipeline creates the PR automatically. For frontend and screenshots, open the PR after committing. Both the `/docs` URL and the terminal `/docs` panel will show updated content within minutes of the merge.

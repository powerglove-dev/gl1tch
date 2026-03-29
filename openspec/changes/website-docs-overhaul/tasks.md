## 1. Screenshots

- [ ] 1.1 Capture `switchboard.png` — full ORCAI switchboard showing pipelines, agent runner, inbox, cron panels
- [ ] 1.2 Capture `agent-modal.png` — agent modal with provider/model/prompt/use_brain/cwd/schedule fields visible
- [ ] 1.3 Capture `theme-picker.png` — theme picker overlay showing full theme list
- [ ] 1.4 Capture `cron.png` — cron jobs screen with log output panel
- [ ] 1.5 Capture `jump-window.png` — jump-to-window overlay showing sysop + active jobs columns
- [x] 1.6 Place all PNGs in `site/public/screenshots/` (directory created; awaiting screenshot files)

## 2. Visual Parity — Site Styling

- [x] 2.1 Audit `site/src/layouts/Base.astro` CSS custom properties; update `--bg` to Apprentice `#262626`
- [x] 2.2 Update all color tokens to Apprentice palette (`--fg #bcbcbc`, `--green #87af87`, `--cyan #87afaf`, `--yellow #d7af5f`, `--comment #585858`, etc.)
- [x] 2.3 Update panel headers to use double-line box-drawing chars matching TUI panel borders
- [x] 2.4 Status bar colors updated (accent → `--green` for current screen)
- [x] 2.5 Nav bar updated to use `--green` as primary accent matching Apprentice theme

## 3. Nav — Add Labs and Docs

- [x] 3.1 Add `{ label: '[8] LABS', screen: 'labs' }` entry to `Nav.astro` navItems array
- [x] 3.2 Add `{ label: '[9] DOCS', screen: 'docs' }` entry to `Nav.astro` navItems array
- [x] 3.3 Register `8` and `9` key bindings in `KeyboardRouter` (screenOrder array extended)
- [x] 3.4 Update `HelpOverlay.astro` to list `8 → LABS` and `9 → DOCS` in the key binding table

## 4. Home Screen Overhaul

- [x] 4.1 Replace existing HomeScreen hero copy with hacker-voice taglines (≥3 `>` prompt-style lines)
- [x] 4.2 Add feature highlight grid: PIPELINES · AGENT RUNNER · BRAIN CONTEXT · THEMES · KEYBOARD NAV
- [x] 4.3 Add OLLAMA SUPERCHARGER callout block with "no API key required" copy and example model names (`qwen2.5-coder`, `llama3.2`)
- [x] 4.4 Embed `switchboard.png` screenshot on Home screen with BBS frame and caption

## 5. Getting Started Overhaul

- [x] 5.1 Restructure `GettingStartedScreen.astro` into 5 numbered steps with BBS-bordered section headers
- [x] 5.2 Step 1 — Install: `go install github.com/adam-stokes/orcai@latest`, note `$GOPATH/bin`
- [x] 5.3 Step 2 — Plugins: list claude, ollama, gh, jq with config snippet showing how to add a plugin wrapper
- [x] 5.4 Step 3 — Ollama: `ollama pull qwen2.5-coder:latest`, `ollama serve`, Ollama supercharger callout block (styled distinctly, "no API key required")
- [x] 5.5 Step 4 — First pipeline: `orcai pipeline run brain-ollama-e2e`, explain switchboard pipeline panel, expected output
- [x] 5.6 Step 5 — First agent: open agent modal (Enter on switchboard), select Ollama, pick model, enter prompt, submit with Ctrl+S
- [x] 5.7 Embed `agent-modal.png` screenshot adjacent to Step 5 with BBS frame

## 6. Labs Screen

- [x] 6.1 Create `site/src/components/screens/LabsScreen.astro`
- [x] 6.2 Define labs data array in the component frontmatter with 5 labs (anonymized)
- [x] 6.3 Render each lab with: title, description, prerequisites list, annotated YAML snippet, `orcai pipeline run <name>` run command
- [x] 6.4 Add `j`/`k` navigation hint status line at bottom of Labs screen
- [x] 6.5 Mount `<section data-screen="labs">` in `index.astro` and import `LabsScreen`

## 7. Docs Screen

- [x] 7.1 Create `site/src/components/screens/DocsScreen.astro`
- [x] 7.2 Write `use_brain` reference section: definition of `use_brain: true` and `write_brain: true`, step-level override (`use_brain: false`)
- [x] 7.3 Write annotated YAML example 1: two-step brain feedback loop (step 1 `write_brain`, step 2 `use_brain`)
- [x] 7.4 Write annotated YAML example 2: pipeline-level `use_brain: true` with one step suppressing via `use_brain: false`
- [x] 7.5 Write "Brain Workspace Tips" section with ≥4 tips (when to write brain notes, when to suppress, naming strategies, multi-model handoff patterns)
- [x] 7.6 Write Pipeline YAML quick-reference table covering: `name`, `version`, `use_brain`, `write_brain`, `steps[].id`, `steps[].executor`/`plugin`, `steps[].model`, `steps[].prompt`
- [x] 7.7 Mount `<section data-screen="docs">` in `index.astro` and import `DocsScreen`

## 8. Verification

- [x] 8.1 Run `npm run build` in `site/` — zero errors, zero broken imports ✓
- [ ] 8.2 Open site locally (`npm run dev`) and verify all 9 screens reachable by number keys 1–9
- [ ] 8.3 Verify screenshots load correctly on Home and Getting Started screens
- [ ] 8.4 Verify Labs screen shows all 5 labs with YAML snippets
- [ ] 8.5 Verify Docs screen shows use_brain reference, 2 YAML examples, tips, and field reference
- [ ] 8.6 Verify help overlay lists `8 → LABS` and `9 → DOCS`
- [ ] 8.7 Run `npm run preview` (or equivalent) and do a final visual check against screenshots of the TUI for color/style parity

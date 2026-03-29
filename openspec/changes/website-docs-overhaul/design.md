## Context

The ORCAI site is an Astro static site using a custom BBS/terminal aesthetic (Dracula palette, monospace fonts, box-drawing characters). Navigation is keyboard-driven via a `KeyboardRouter` JS module — number keys switch between screens that are pre-rendered as hidden `<section data-screen="...">` divs revealed on demand. Components live in `site/src/components/screens/`. The existing site has 7 screens (home, about, getting-started, plugins, pipelines, changelog, themes).

This overhaul adds 2 new screens (labs, docs), rewrites 2 existing screens (home, getting-started), and introduces screenshots and lab content. No build system changes are needed — Astro handles everything.

## Goals / Non-Goals

**Goals:**
- Add LabsScreen and DocsScreen as new keyboard-accessible screens ([8] and [9])
- Rewrite HomeScreen with hacker-voice marketing copy and Ollama callout
- Expand GettingStartedScreen into a full multi-step walkthrough (5 steps)
- Document `use_brain` / `write_brain` with examples and tips in DocsScreen
- Embed real TUI screenshots with BBS-styled frames
- Add 4–6 anonymized lab pipeline examples with annotations

**Non-Goals:**
- Server-side rendering, search, or authentication
- Changing the Go codebase or pipeline runtime
- Adding a CMS or database backend
- Changing the existing Dracula/BBS visual design system
- Internationalisation

## Decisions

### D1: Screens-as-divs pattern — extend, don't replace
The existing site renders all screens as hidden divs and `KeyboardRouter` reveals them. Adding new screens follows the same pattern: add `<section data-screen="...">` in `index.astro`, import the new screen component, add a nav entry in `Nav.astro`.

**Alternative considered**: Separate Astro pages per screen with client-side routing. Rejected — it would require a router library, break the BBS single-page feel, and add complexity with no benefit.

### D2: Screenshots as static PNGs in `public/screenshots/`
TUI screenshots captured with macOS screencapture/tmux are placed in `public/screenshots/`. They are rendered with `<img>` tags inside styled BBS frames (border-drawing chars, a caption line). No build-time image optimization needed for the target audience (devs who already load monospace fonts).

**Alternative considered**: ANSI art reproductions of the screenshots. Would be more authentic but extremely labor-intensive and would not convey the actual visual appearance to new users.

### D3: Labs as inline Astro component data, not MDX content collection
Lab content (title, description, YAML snippet, prerequisites) is defined directly in `LabsScreen.astro` as a typed array. This avoids standing up a content collection for a small, stable data set.

**Alternative considered**: `site/src/content/labs/*.yaml` content collection. Appropriate if labs grow to 20+; overkill for the initial 5–6.

### D4: `use_brain` docs live in DocsScreen, not a separate page
DocsScreen covers three topics: use_brain reference, pipeline YAML quick-ref, and workspace tips. These are complementary and navigation is already the primary discovery path for this audience. A separate page would dilute keyboard-nav cohesion.

### D5: Nav numbering — [8] LABS, [9] DOCS
Existing keys 1–7 are locked in. Labs and Docs are additive. The `[?]` help overlay remains accessible via `?` key regardless.

## Risks / Trade-offs

- **Long Getting Started screen scroll** → Mitigation: section it with collapsible ANSI-bordered panels; user can arrow-key through within the screen.
- **Screenshot staleness** — PNGs will drift from the real UI over time → Mitigation: note in the file that screenshots are illustrative; plan to re-capture after major UI changes.
- **Nav bar overflow on narrow viewports** → Mitigation: Nav already wraps; existing CSS handles it; test at 1024px wide.
- **Labs YAML snippets may bitrot** — pipeline syntax evolves → Mitigation: labs show minimal, schema-stable YAML; add a comment `# see docs for full schema`.

## Migration Plan

1. Capture TUI screenshots (5 images) and place in `site/public/screenshots/`
2. Write new screen components (LabsScreen, DocsScreen)
3. Rewrite HomeScreen and GettingStartedScreen
4. Update Nav.astro and index.astro
5. Run `npm run build` in `site/` and verify no broken imports
6. Deploy via existing GitHub Pages workflow (no infra changes)
7. Rollback: revert the affected `.astro` files; no data migration needed

## Open Questions

- Should we add a `[0] LABS` shortcut or keep sequential numbering? (Decision deferred — use [8] for now)
- Screenshots: should they link to a full-size lightbox? (Defer — keep simple `<img>` for now)

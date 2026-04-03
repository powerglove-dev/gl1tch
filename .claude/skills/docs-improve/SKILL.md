---
name: docs-improve
description: Run the gl1tch docs-improve pipeline to write or improve documentation for a specific feature and open a PR. Use this skill whenever the user says anything like "update docs for this section", "document this", "write docs for X", "improve the docs", "PR the docs", or mentions wanting documentation improved for a feature — even if they don't say "docs-improve" explicitly. Trigger immediately without asking for confirmation.
---

The docs-improve pipeline handles everything: it picks the right file, writes the doc, runs a principal engineer review pass, checks TUI test coverage, commits, and opens a PR. Your job is to extract a clear focus, hand it off to the pipeline, and then — if the pipeline reports missing integration tests — write those tests before the PR is reviewed.

## The two goals of gl1tch documentation

Every doc page exists for exactly one of these reasons:

1. **Get running fast** — a new user should be running their first pipeline in under 5 minutes. Quickstart, console, pipelines, examples, and brain are the highest-value pages.
2. **Make it yours** — an experienced user should be able to customize everything: their AI providers, pipelines, themes, saved prompts, scheduled automations, and plugins.

Internal architecture, implementation details, and infrastructure choices (tmux, BubbleTea, OTel, Go packages) are not user goals. The pipeline is instructed to keep them out of the docs unless the user literally needs to type a command involving them.

## How docs reach the two public surfaces

All docs live in `site/src/content/pipelines/*.md`. Both surfaces read from this same Astro content collection at build time:

- **`/docs` URL** (`site/src/pages/docs/`) — card grid linking to individual pages
- **Terminal `/docs` command** — inline sidebar+viewer shown when `/docs` is typed on the homepage

The site is a static Astro build. After docs are updated, the site must be rebuilt for either surface to show the new content.

**CI/CD path (production):** `gh-pages.yml` triggers on any push to `site/**` and rebuilds + deploys automatically. After a PR is merged to main, both surfaces will show the updated docs within minutes.

**Local preview:**
```bash
cd /Users/stokes/Projects/gl1tch/site && npm run build
```
Then open `site/dist/` or run `task site:dev` to browse locally.

## Steps

1. **Extract the focus.** Look at what the user is asking about:
   - Explicit topic: "update docs for cron" → `"cron scheduling"`
   - Referring to current context: "update docs for this section" → infer from the file, function, or topic being discussed in the conversation
   - No topic: "improve the docs" with no context → run without `--input` and let the pipeline pick the highest-priority gap

2. **Run the pipeline:**
   ```bash
   cd /Users/stokes/Projects/gl1tch && glitch pipeline run docs-improve --input "<focus>"
   ```
   If there's no specific focus, omit `--input` entirely.

3. **Check the pipeline outcome.** Two paths:
   - **`AUTO_MERGED:`** — coverage was clean, the PR was squash-merged and the branch deleted automatically. CI/CD will rebuild the site; both surfaces will show fresh docs automatically.
   - **`MISSING_TESTS:`** — the PR is open but has test gaps that must be closed before merging (see step 4).

4. **Write the missing tests** (when `MISSING_TESTS:` appears). For each command listed:

   a. Read `internal/console/tui_integration_test.go` and the relevant source in `internal/console/` to understand the expected output for that command.

   b. Add one or more test functions following the established pattern:
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
   Use `isTUIAlive` at the end of every test. Look at what the handler actually returns (not guessed strings) — grep the `internal/console/` source for the command handler's response text.

   c. **Commit the tests to the PR branch the pipeline just created:**
   ```bash
   cd /Users/stokes/Projects/gl1tch
   BRANCH=$(git branch --list "docs/improve-*" --sort=-creatordate | head -1 | tr -d ' *')
   git checkout "$BRANCH"
   git add internal/console/tui_integration_test.go
   git commit -m "test(console): add tmux integration tests for documented TUI actions"
   git push
   git checkout main
   ```

5. **Report the result.** Surface the PR URL. If tests were written, mention which commands are now covered. Remind the user that after the PR merges, CI/CD rebuilds the site and both the `/docs` URL and the terminal `/docs` panel will show the fresh content automatically.

## Focus phrasing tips

Lean toward what the user DOES with the feature, not the feature's internal name:
- "cron" → `"scheduling pipelines to run automatically"`
- "brain" → `"gl1tch remembering context across sessions"`
- "console" → `"the interactive AI assistant"`
- "plugins" → `"installing and building custom pipelines"`
- "themes" → `"customizing the look of gl1tch"`

The topic catalogue in the pipeline maps these to the right file. Don't overthink the phrasing — the pipeline's pick step handles ambiguity.

## What the pipeline avoids

The `write_doc` and `verify` steps are explicitly instructed to:
- Not mention tmux, BubbleTea, robfig/cron, OpenTelemetry, or Go package names in user-facing pages
- Not lead with how something is built — always lead with what the user can do
- Use "your" framing throughout: your assistant, your pipelines, your brain
- Put a working example before any explanation

If you review a PR from this pipeline and see internal implementation details surfaced to users, flag it and re-run the pipeline with a more specific focus.

## Why tests matter

gl1tch docs tell users exactly what to type. If a page says "run `/foo` to list items" but `/foo` has no integration test, there is no guarantee the behaviour described is real or will stay real. The tmux integration tests in `internal/console/` are the single source of truth that documented TUI workflows actually work. Every documented slash command must have at least one `TestTmux_Cmd_*` covering it.

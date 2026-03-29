## 1. Taskfile Setup

- [x] 1.1 Install Task v3 via `go install github.com/go-task/task/v3/cmd/task@latest` and add to `tools.go` or document in README
- [x] 1.2 Create `Taskfile.yml` at project root with `version: "3"` and all tasks mirroring current Makefile: `build`, `install`, `test`, `clean`
- [x] 1.3 Add `run` task (non-destructive: starts orcai without wiping db/config)
- [x] 1.4 Add `run:clean` task (destructive: kills tmux sessions, removes db/config, starts fresh — mirrors old `make run`)
- [x] 1.5 Add `debug`, `debug:connect`, and `debug:tmux` tasks mirroring current Makefile debug targets
- [x] 1.6 Add `release:snapshot` task that runs `goreleaser release --snapshot --clean`
- [x] 1.7 Add `release:tag` task that accepts a `VERSION` variable, creates a signed annotated git tag, and prints instructions to push
- [x] 1.8 Add descriptions to all tasks so `task --list` is self-documenting
- [x] 1.9 Delete `Makefile` after verifying all tasks work

## 2. GoReleaser Configuration

- [x] 2.1 Add `goreleaser` to local toolchain (document install via `go install github.com/goreleaser/goreleaser/v2@latest` or homebrew)
- [x] 2.2 Create `.goreleaser.yml` at project root with `version: 2` header
- [x] 2.3 Configure `builds` section: targets `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`; set `CGO_ENABLED=0`; inject `-ldflags` for `main.version`, `main.commit`, `main.date`
- [x] 2.4 Add `main.version`, `main.commit`, `main.date` variables to `main.go` and wire `--version` flag to print them
- [x] 2.5 Configure `archives` section: `.tar.gz` for POSIX, `.zip` for Windows
- [x] 2.6 Configure `checksum` section with `name_template: checksums.txt` and `sha256` algorithm
- [x] 2.7 Configure `changelog` section: use `conventional_commits` grouping; include `feat`, `fix`, `perf`, `refactor`; exclude `chore`, `docs`, `ci`, `test`
- [x] 2.8 Add `dist/` to `.gitignore`
- [x] 2.9 Run `task release:snapshot` locally and verify all 5 platform archives appear in `dist/`

## 3. GitHub Actions — Snapshot Workflow

- [x] 3.1 Create `.github/workflows/snapshot.yml` triggered on `push: branches: [main]`
- [x] 3.2 Add `permissions: contents: read` block
- [x] 3.3 Set up Go using `go-version-file: go.mod`
- [x] 3.4 Install Task via `arduino/setup-task@v2` or equivalent
- [x] 3.5 Run `goreleaser release --snapshot --clean` via `goreleaser/goreleaser-action@v6`
- [x] 3.6 Upload `dist/` as a workflow artifact with 7-day retention
- [x] 3.7 Verify snapshot workflow triggers on a test push and produces artifacts

## 4. GitHub Actions — Release Workflow

- [x] 4.1 Create `.github/workflows/release.yml` triggered on `push: tags: ['v[0-9]+.[0-9]+.[0-9]+']`
- [x] 4.2 Add `permissions: contents: write` block (for GitHub Release creation and changelog commit)
- [x] 4.3 Set up Go using `go-version-file: go.mod`
- [x] 4.4 Install Task
- [x] 4.5 Run `goreleaser release --clean` via `goreleaser/goreleaser-action@v6` with `GITHUB_TOKEN`
- [x] 4.6 After GoReleaser succeeds, add a step that reads the generated `CHANGELOG.md` and creates `site/src/content/changelog/v${TAG}.md` with frontmatter (`version`, `date`, `highlights`, `breaking: false`) parsed from the changelog
- [x] 4.7 Add a step that commits the new changelog file to `main` (skip if file already exists) using `peter-evans/create-pull-request` or a direct `git push` with bot credentials
- [x] 4.8 Verify full release workflow end-to-end by tagging `v0.1.0` on a test branch

## 5. Astro Site — Changelog Collection

- [x] 5.1 Add `changelog` collection to `site/src/content.config.ts` with Zod schema: `version` (string), `date` (date), `highlights` (string array), `breaking` (boolean, default false)
- [x] 5.2 Create `site/src/content/changelog/` directory with an initial `v0.1.0.md` entry (hand-authored highlights for the inaugural release)
- [x] 5.3 Create `site/src/pages/changelog/index.astro` that fetches all changelog entries, sorts by version descending, and renders version, date, breaking badge, and highlight bullets
- [x] 5.4 Apply existing Dracula/ABBS CSS classes and monospace styling — no new color variables or font families
- [x] 5.5 Add "Changelog" link to the site's primary navigation component
- [x] 5.6 Run `npm run build` in `site/` and verify no type or schema errors
- [x] 5.7 Visually review the `/changelog` page in local dev (`npm run dev`) for aesthetic consistency with the rest of the site

## 6. Release Skill

- [x] 6.1 Create `.claude/skills/release.md` as a Claude Code skill following the existing skill format in the project
- [x] 6.2 Skill step 1: verify working tree is clean and current branch is up-to-date with `main`
- [x] 6.3 Skill step 2: determine next semver version by reading latest git tag and prompting user for major/minor/patch bump
- [x] 6.4 Skill step 3: create a release branch (`release/v{VERSION}`), run final `task test`, open a PR to `main`
- [x] 6.5 Skill step 4: instruct developer to wait for all CI checks to pass on the PR, then merge
- [x] 6.6 Skill step 5: after merge, generate the `site/src/content/changelog/v{VERSION}.md` file locally using GoReleaser changelog output, pause for developer to curate highlights
- [x] 6.7 Skill step 6: commit the changelog file, push to `main`, then create and push the semver tag (`git tag -s v{VERSION}`) to trigger the release workflow
- [x] 6.8 Skill step 7: link developer to the GitHub Actions release workflow run to monitor progress
- [x] 6.9 Add a "Releasing" section to `README.md` pointing to `/release` skill as the canonical release process

## 7. Smoke Test End-to-End

- [ ] 7.1 Run full release via `/release` skill targeting `v0.1.0`
- [ ] 7.2 Confirm GitHub Release page has all 5 platform archives and `checksums.txt`
- [ ] 7.3 Download and run the `darwin/arm64` binary and verify `orcai --version` outputs correct version, commit, and date
- [ ] 7.4 Confirm the ORCAI site's `/changelog` page shows the `v0.1.0` entry after deploy

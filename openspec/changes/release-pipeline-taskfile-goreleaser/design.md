## Context

ORCAI currently uses a hand-rolled Makefile for all dev and build tasks. There is no automated release pipeline, no multi-arch binary distribution, and no versioning strategy beyond git commits. The project is a Go BubbleTea TUI targeting hackers, sysadmins, and AI enthusiasts who expect to install a binary directly from GitHub Releases for their platform.

The website (Astro, deployed via `gh-pages.yml`) is manually maintained with no changelog. There is an existing `website-docs-overhaul` spec.

## Goals / Non-Goals

**Goals:**
- Replace Makefile with Taskfile as the single task runner for dev, build, test, debug, and release operations
- Ship GoReleaser-produced binaries for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` on every versioned tag
- Automate snapshot (dev) builds on every push to `main` via GitHub Actions
- Automate production releases on semver tags via GitHub Actions
- Auto-generate and publish a curated changelog page on the ORCAI site per release
- Make version bumping scriptable and part of the release workflow

**Non-Goals:**
- Homebrew tap or other package manager distribution (can be added later)
- Container image publishing (Dockerfile exists but is out of scope here)
- Signing binaries with GPG/cosign (deferred to a hardening pass)
- Replacing `go install` for local developer workflow — Taskfile wraps it

## Decisions

### Task (Taskfile) over alternatives

**Decision**: Use [taskfile.dev](https://taskfile.dev) (Task v3) as the Makefile replacement.

**Rationale**: Task is the most widely adopted modern task runner in the Go ecosystem (used by projects like Hugo, Mage, and hundreds of open-source Go tools). It offers: native Go template variables, dependency graphs between tasks, cross-platform support (no bash required for task definitions), dotenv support, `--watch` mode, and excellent CLI help output. Alternatives considered:

- `Mage` — Go-native but requires compiling a magefile; less discoverable for contributors
- `just` — Excellent but Rust-based and less common in Go ecosystems
- `make` (enhanced) — Portable but fundamentally limited; no DAG, poor Windows support

### GoReleaser for distribution

**Decision**: Use GoReleaser v2 with a `.goreleaser.yml` at the project root.

**Rationale**: GoReleaser is the de-facto standard for Go binary distribution. It handles cross-compilation via `GOOS`/`GOARCH`, archive creation (`.tar.gz` / `.zip`), checksum files, GitHub Release creation, and changelog injection. It integrates natively with GitHub Actions via the `goreleaser/goreleaser-action`.

### Version strategy: conventional commits + git tags

**Decision**: Version is driven by semver git tags (`v1.2.3`). Dev snapshots use `--snapshot` mode. Version string is injected at build time via `-ldflags`.

**Rationale**: Simple and explicit. No automated version bumping magic that surprises contributors. A `task release:tag` command wraps `git tag -s` to guide the process.

### Changelog: per-release markdown files in Astro content collection

**Decision**: Each release gets a `site/src/content/changelog/v{version}.md` file with frontmatter (`version`, `date`, `highlights[]`, `breaking: bool`). The Astro page renders them sorted by version descending.

**Rationale**: Keeps changelog content in version control alongside the code, easy to edit/curate before tagging, and the Astro content collection gives type-safe frontmatter. The release workflow creates the changelog file from the GoReleaser-generated `CHANGELOG.md`, then opens a PR (or auto-pushes to main) so the site deploys automatically.

### GitHub Actions workflow split

**Decision**: Two separate workflow files:
- `snapshot.yml` — triggers on push to `main`; runs `goreleaser release --snapshot --clean`; uploads artifacts to the workflow run (not a GitHub Release)
- `release.yml` — triggers on `push: tags: ['v*.*.*']`; runs full `goreleaser release`; creates GitHub Release; triggers site changelog update

**Rationale**: Separating concerns keeps the snapshot workflow fast and the release workflow auditable. Dev builds are available for testing without polluting the releases page.

## Risks / Trade-offs

- **GoReleaser free tier limits** → The `.goreleaser.yml` will use only OSS-compatible features (no Docker, no Homebrew tap automation). Mitigation: document which features require GoReleaser Pro if desired later.
- **Windows cross-compilation** → CGO must be disabled (`CGO_ENABLED=0`) for Windows targets. If any dependency requires CGO, windows builds will fail. Mitigation: audit `go.mod` for CGO dependencies; add `CGO_ENABLED=0` assertion to CI.
- **Changelog curation burden** → Auto-generated changelogs from commit messages are noisy. Mitigation: enforce conventional commits (`feat:`, `fix:`, `chore:`) in the GoReleaser changelog config so only meaningful commits surface; keep a manual override path.
- **Site auto-push on release** → The release workflow pushing to `main` triggers the existing `gh-pages.yml` deploy. This is the intended behavior, but adds latency to the release flow (~2 min). Mitigation: document the expected sequence; no action required.

## Migration Plan

1. Add `Taskfile.yml` with all tasks mirroring current Makefile targets plus new release tasks
2. Verify `task build`, `task test`, `task run`, `task debug` work identically to current `make` equivalents
3. Delete `Makefile`
4. Add `.goreleaser.yml` and test locally with `task release:snapshot`
5. Add `.github/workflows/snapshot.yml` and `.github/workflows/release.yml`
6. Add Astro changelog collection schema and placeholder `v0.1.0.md` entry
7. Tag `v0.1.0` to smoke-test the full release pipeline end-to-end

## Open Questions

- Should `task run` reset the database by default (current Makefile behavior) or make that a separate `task run:clean` target? Preference: split them so normal `task run` is non-destructive.
- What is the desired version for the first tag (`v0.1.0` vs `v1.0.0`)? Defaulting to `v0.1.0` to signal pre-stable.

## Why

The project's Makefile has grown into a fragile, platform-specific shell script that lacks cross-platform support, dependency management, and discoverability. We need a professional release pipeline so ORCAI binaries can be distributed to hackers and AI enthusiasts across Linux, macOS, and Windows without manual intervention.

## What Changes

- **Replace Makefile** with [Task](https://taskfile.dev) (Taskfile) — the most widely adopted, feature-rich task runner for Go projects, with YAML-based configuration, built-in dependency graphs, variable interpolation, and cross-platform support
- **Add GoReleaser** for automated multi-arch/multi-OS binary builds, archive generation, checksums, and GitHub Release artifact publishing
- **Add GitHub Actions workflows** for:
  - Automated version tagging on merge to `main` (dev/snapshot builds)
  - Full release workflow triggered by semver tags (production artifacts)
  - Changelog generation integrated into the release pipeline
- **Add changelog page to the Astro site** — a curated, human-readable changelog surfacing highlights that matter to hackers and AI enthusiasts (new AI integrations, new TUI capabilities, protocol changes, plugin APIs, etc.), auto-updated by the release workflow on each tag

## Capabilities

### New Capabilities

- `task-runner`: Replace Makefile with Taskfile; all dev, build, test, debug, and release tasks defined in `Taskfile.yml` with dependency graphs and cross-platform support
- `goreleaser-build`: GoReleaser configuration for multi-arch (amd64, arm64) × multi-OS (linux, darwin, windows) binary builds, archives, checksums, and GitHub Release uploads
- `release-workflow`: GitHub Actions CI/CD — snapshot builds on every main merge, production release artifacts on semver tags, version bump automation
- `site-changelog`: Astro changelog page on the ORCAI website, auto-populated from release notes, targeting the hacker/AI enthusiast audience with curated highlights per release
- `release-skill`: Claude Code `/release` skill that walks developers through the full PR-to-tag release flow, enforcing protected-main discipline and changelog curation

### Modified Capabilities

- `website-docs-overhaul`: The website gains a new `/changelog` section; nav and content config updated to surface it

## Impact

- `Makefile` — removed; replaced by `Taskfile.yml`
- `.github/workflows/` — new workflows added: `snapshot.yml`, `release.yml`
- `.goreleaser.yml` — new file at project root
- `site/src/content/` — new `changelog/` collection with per-release markdown entries
- `site/src/pages/changelog/` — new Astro page(s) rendering changelog entries
- `site/src/content.config.ts` — updated with changelog collection schema
- No changes to Go source, protobuf, or internal packages

## ADDED Requirements

### Requirement: Snapshot workflow runs on every push to main
A GitHub Actions workflow `snapshot.yml` SHALL trigger on every push to the `main` branch and produce snapshot build artifacts for testing. It SHALL NOT create a GitHub Release.

#### Scenario: Push to main triggers snapshot build
- **WHEN** a commit is pushed to `main`
- **THEN** the `snapshot` workflow runs `goreleaser release --snapshot --clean` and uploads `dist/` as a workflow artifact

#### Scenario: Snapshot does not create a GitHub Release
- **WHEN** the snapshot workflow completes successfully
- **THEN** no GitHub Release is created or modified

### Requirement: Release workflow runs on semver tag push
A GitHub Actions workflow `release.yml` SHALL trigger on push of tags matching `v*.*.*` and produce a full GitHub Release with all platform artifacts.

#### Scenario: Semver tag triggers release workflow
- **WHEN** a tag matching `v[0-9]+.[0-9]+.[0-9]+` is pushed
- **THEN** the `release` workflow runs `goreleaser release --clean`

#### Scenario: GitHub Release is created with artifacts
- **WHEN** the release workflow completes successfully
- **THEN** a GitHub Release exists for the tag with all platform archives and `checksums.txt` attached

#### Scenario: Release workflow fails on dirty working tree
- **WHEN** the release workflow runs with uncommitted changes in the working tree
- **THEN** GoReleaser exits with a non-zero code and the workflow is marked failed

### Requirement: Release workflow updates the site changelog
After a successful GoReleaser run, the release workflow SHALL create or update `site/src/content/changelog/v{VERSION}.md` and push it to `main`, triggering the existing `gh-pages.yml` site deploy.

#### Scenario: Site changelog file is created on release
- **WHEN** the release workflow completes and a new tag is published
- **THEN** a commit is pushed to `main` adding `site/src/content/changelog/v{VERSION}.md` with version metadata and highlights extracted from the GoReleaser changelog

#### Scenario: Existing changelog file is not overwritten
- **WHEN** a changelog file for the same version already exists in `main`
- **THEN** the release workflow skips the changelog commit step

### Requirement: Workflows use least-privilege GitHub token permissions
Both workflow files SHALL declare explicit `permissions` blocks granting only the minimum required scopes (`contents: write` for release creation, `contents: read` for snapshot).

#### Scenario: Snapshot workflow uses read-only token
- **WHEN** the snapshot workflow runs
- **THEN** the GITHUB_TOKEN has `contents: read` and no write access to releases

#### Scenario: Release workflow uses write token scoped to contents
- **WHEN** the release workflow runs
- **THEN** the GITHUB_TOKEN has `contents: write` (for tag and release) and no other elevated permissions

### Requirement: Go and Task are set up consistently across workflows
Both workflows SHALL install the same pinned Go version from `go.mod` and install Task via the official setup action before running any build steps.

#### Scenario: Go version matches go.mod
- **WHEN** either workflow sets up Go
- **THEN** the version is read from `go.mod` using `go-version-file: go.mod`

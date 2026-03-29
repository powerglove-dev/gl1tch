## ADDED Requirements

### Requirement: GoReleaser produces multi-platform binaries
The project SHALL include a `.goreleaser.yml` that configures GoReleaser v2 to build `orcai` binaries for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`.

#### Scenario: GoReleaser builds all targets
- **WHEN** `goreleaser release --clean` is invoked with a valid git tag
- **THEN** binaries for all 5 platform/arch combinations are produced in `dist/`

#### Scenario: GoReleaser builds a snapshot
- **WHEN** `goreleaser release --snapshot --clean` is invoked
- **THEN** all platform binaries are produced with a snapshot version suffix and no GitHub Release is created

### Requirement: GoReleaser produces archives and checksums
For each platform, GoReleaser SHALL package the binary into a `.tar.gz` archive (Linux/macOS) or `.zip` (Windows) and produce a `checksums.txt` file covering all archives.

#### Scenario: Archives are created for POSIX platforms
- **WHEN** a release build completes
- **THEN** `dist/` contains `orcai_<version>_linux_amd64.tar.gz`, `orcai_<version>_darwin_arm64.tar.gz`, etc.

#### Scenario: Archive is created for Windows
- **WHEN** a release build completes
- **THEN** `dist/` contains `orcai_<version>_windows_amd64.zip`

#### Scenario: Checksum file is produced
- **WHEN** a release build completes
- **THEN** `dist/checksums.txt` contains SHA256 hashes for every archive

### Requirement: Version string is injected at build time
GoReleaser SHALL inject the version, commit, and build date into the binary via `-ldflags` targeting `main.version`, `main.commit`, and `main.date` variables.

#### Scenario: Binary reports version
- **WHEN** a user runs `orcai --version`
- **THEN** the output includes the semver tag, short commit SHA, and build date

### Requirement: CGO is disabled for all release builds
All GoReleaser build targets SHALL set `CGO_ENABLED=0` to ensure static binaries that work without shared libraries.

#### Scenario: Linux binary runs without glibc dependency
- **WHEN** the `linux/amd64` binary is copied to a minimal Alpine container
- **THEN** it executes without missing library errors

### Requirement: GoReleaser generates a changelog from conventional commits
The `.goreleaser.yml` changelog configuration SHALL filter commits by conventional commit prefixes (`feat`, `fix`, `perf`, `refactor`) and exclude `chore`, `docs`, and `ci` commits from the release notes.

#### Scenario: Release notes contain only meaningful changes
- **WHEN** a GitHub Release is created by GoReleaser
- **THEN** the release body lists only `feat:`, `fix:`, `perf:`, and `refactor:` commits since the last tag

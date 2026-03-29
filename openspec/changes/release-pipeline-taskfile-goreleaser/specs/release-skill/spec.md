## ADDED Requirements

### Requirement: A Claude Code skill guides developers through the release PR process
The project SHALL include a Claude Code skill at `.claude/skills/release.md` that walks a developer through the complete release flow: branch → PR → merge to protected main → semver tag → release workflow trigger.

#### Scenario: Developer invokes /release skill
- **WHEN** a developer runs `/release` in Claude Code
- **THEN** the skill guides them step-by-step: verify clean main, create a release branch, open a PR, confirm CI is green, merge, then tag with the target semver version

#### Scenario: Skill enforces PR-before-tag discipline
- **WHEN** the release skill is followed
- **THEN** the developer MUST merge a PR to main before tagging — the skill SHALL NOT instruct direct pushes to main or tagging from a feature branch

#### Scenario: Skill prompts for version selection
- **WHEN** the skill reaches the tagging step
- **THEN** it asks the developer to choose a version bump type (major/minor/patch) and computes the next semver from the latest git tag

#### Scenario: Skill checks that main is green before tagging
- **WHEN** the developer is about to create the release tag
- **THEN** the skill instructs them to verify that all GitHub Actions checks on main are passing before proceeding

### Requirement: Release skill documents the changelog curation step
The release skill SHALL include a step for the developer to review and edit the auto-generated `site/src/content/changelog/v{VERSION}.md` before it is committed, ensuring highlights are curated for the hacker/AI audience.

#### Scenario: Changelog is reviewed before release
- **WHEN** the skill generates the changelog file from GoReleaser output
- **THEN** it pauses and prompts the developer to review/edit the highlights before the final commit and tag push

### Requirement: Release skill is listed in the project's skill index
The release skill SHALL be discoverable via `task --list` documentation and referenced in `README.md` under a "Releasing" section.

#### Scenario: New contributor finds the release process
- **WHEN** a contributor looks at README.md for how to cut a release
- **THEN** they find a "Releasing" section that says to run `/release` in Claude Code

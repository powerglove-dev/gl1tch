## ADDED Requirements

### Requirement: Astro site has a changelog content collection
The site SHALL define a `changelog` content collection in `site/src/content.config.ts` with a Zod schema covering: `version` (string, semver), `date` (date), `highlights` (string array), and `breaking` (boolean, default false).

#### Scenario: Valid changelog entry passes schema validation
- **WHEN** a markdown file in `site/src/content/changelog/` has valid frontmatter
- **THEN** Astro builds without type errors

#### Scenario: Missing required field causes build error
- **WHEN** a changelog file is missing the `version` field
- **THEN** `npm run build` exits with a content schema validation error

### Requirement: Site has a /changelog index page
The Astro site SHALL include a `site/src/pages/changelog/index.astro` page that renders all changelog entries sorted by version descending, listing version, date, breaking flag, and highlights for each release.

#### Scenario: Changelog page lists all releases
- **WHEN** a visitor navigates to `/changelog`
- **THEN** all changelog entries are displayed, newest first, with version, date, and highlight bullets

#### Scenario: Breaking change is visually flagged
- **WHEN** a changelog entry has `breaking: true`
- **THEN** the entry is visually distinguished (e.g., a "BREAKING" badge) on the changelog page

### Requirement: Changelog entries target hacker and AI enthusiast audience
Each changelog entry SHALL lead with highlights that matter to the target audience: new AI integrations, new TUI capabilities, agent pipeline features, plugin API changes, and protocol updates. Dependency bumps, linting fixes, and CI housekeeping SHALL NOT appear as highlights.

#### Scenario: Highlights focus on user-facing capabilities
- **WHEN** a new changelog entry is authored
- **THEN** each item in `highlights[]` describes a user-visible or developer-facing capability change, not an internal implementation detail

### Requirement: Site changelog page matches ABBS Dracula aesthetic
The changelog page SHALL use the existing site design system (Dracula palette, monospace fonts, ANSI-inspired visual language) and SHALL NOT introduce new color variables or font families.

#### Scenario: Changelog page uses existing CSS classes
- **WHEN** the changelog page is rendered
- **THEN** it uses only color variables and component patterns already present in the site's design system

### Requirement: Site navigation includes a link to /changelog
The site's primary navigation SHALL include a "Changelog" link pointing to `/changelog`.

#### Scenario: Changelog is reachable from the nav
- **WHEN** a visitor is on any page of the site
- **THEN** the nav bar contains a link to `/changelog`

## ADDED Requirements

### Requirement: TUI screenshots embedded in site
The site SHALL display real PNG screenshots of the ORCAI TUI, framed with BBS box-drawing characters and a caption, inside the screens where they are contextually relevant (Getting Started, Home, Labs, Docs).

#### Scenario: Screenshot renders inline
- **WHEN** a visitor navigates to a screen containing a screenshot
- **THEN** the `<img>` element SHALL load from `/screenshots/<name>.png` and be visible without scrolling horizontally

#### Scenario: Screenshot has BBS frame
- **WHEN** a screenshot is rendered
- **THEN** it SHALL be wrapped in a `<figure>` with a monospace `<figcaption>` using Dracula palette colors matching the surrounding screen

#### Scenario: Minimum screenshot set
- **WHEN** the site is deployed
- **THEN** `public/screenshots/` SHALL contain at minimum: `switchboard.png`, `agent-modal.png`, `theme-picker.png`, `cron.png`, `jump-window.png`

#### Scenario: Alt text
- **WHEN** a screenshot `<img>` is rendered
- **THEN** it SHALL have a descriptive `alt` attribute describing the screen being shown

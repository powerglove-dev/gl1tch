---
name: site-scaffold
description: Scaffold or regenerate the orcai ABS (Agentic Bulletin System) site at site/src/pages/ — Dracula palette, hex-dump canvas wallpaper, JetBrains Mono, ANSI art hero, dot-separated nav. Use when adding a new page to the Astro site.
disable-model-invocation: true
---

Scaffold or update the orcai ABS (Agentic Bulletin System) site at site/src/pages/.

When invoked with a page name as argument, create that Astro page. With no argument, scaffold a new page stub.

## Site Structure (Astro 6, base: /orcai)

site/src/
  pages/               — Astro pages using BBS.astro layout
  layouts/
    Base.astro         — html/head/body shell, loads bbs.css + bbs.js
    BBS.astro          — Base + Nav + Footer
  components/
    Nav.astro          — Fixed top bar, activePage prop, · separators
    Footer.astro       — [ GLITCH ABS ] [ MIT ] etc.
    SysinfoBox.astro   — CSS-bordered table (no manual char padding)
    FeatureCard.astro  — Feature grid cards
    AnsiLogo.astro     — ANSI block logo
  content/             — Content Collections (changelog, registry, pipelines)
site/public/
  css/bbs.css          — Dracula palette, CRT scanlines, ABS components
  js/bbs.js            — Hex canvas, typewriter, color cycling, keyboard nav

## Aesthetic Rules (ENFORCE ON ALL PAGES)

- Background: #282a36 ONLY. No white, light, or gradient backgrounds.
- Font: JetBrains Mono (Google Fonts) throughout — full box-drawing glyph coverage
- Nav: Written once in Nav.astro, activePage prop sets active link
- Box-drawing: Use ║╔╗╚╝─│┌┐└┘· for all UI frames
- Colors: Only Dracula palette vars (--purple, --pink, --cyan, --green, --yellow, --red, --comment)
- NO Bootstrap, NO Tailwind, NO utility frameworks
- CRT scanlines via CSS ::after on body
- Vignette via CSS ::before on body

## When Adding a New Page

1. Create site/src/pages/<name>.astro using BBS layout:
   ```astro
   ---
   import BBS from '../layouts/BBS.astro';
   ---
   <BBS title="Page Name" activePage="page-key">
     <div class="content content-padded" style="position:relative;z-index:1">
       ...
     </div>
   </BBS>
   ```
2. Add the page key + href to navItems array in site/src/components/Nav.astro
3. Follow ABS content style: ASCII boxes, terminal prompts, monospace tables

## Palette Reference (from sdk/styles/styles.go)

--bg: #282a36 | --fg: #f8f8f2 | --purple: #bd93f9 | --pink: #ff79c6
--cyan: #8be9fd | --green: #50fa7b | --yellow: #f1fa8c | --red: #ff5555
--comment: #6272a4 | --selbg: #44475a | --darkbg: #1e1f29

# Astro BBS Site Migration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate the orcai GitHub Pages site from hand-written HTML to Astro with Content Collections, fixing font/alignment issues and adding changelog, ANSI gallery, pipeline reference, and plugin registry pages.

**Architecture:** Astro static site at `site/`, deployed to `gh-pages` branch via GitHub Actions. Layouts and components replace copy-pasted HTML. Content Collections (Markdown/MDX) drive changelog, plugin registry, and pipeline reference. All existing BBS CSS/JS copied unchanged except font swap to JetBrains Mono.

**Tech Stack:** Astro 4.x, Content Collections (Zod schema), MDX, JetBrains Mono (Google Fonts), GitHub Actions (`peaceiris/actions-gh-pages`)

---

## Task 1: Initialize Astro project

**Files:**
- Create: `site/` (directory)
- Create: `site/astro.config.mjs`
- Create: `site/package.json`
- Create: `site/tsconfig.json`

**Step 1: Scaffold Astro**

```bash
cd /Users/stokes/Projects/orcai
npm create astro@latest site -- --template minimal --no-install --typescript strict --no-git
```

**Step 2: Install dependencies**

```bash
cd site && npm install
npm install @astrojs/mdx
```

**Step 3: Configure Astro**

Replace `site/astro.config.mjs` with:

```js
import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';

export default defineConfig({
  integrations: [mdx()],
  output: 'static',
  base: '/orcai',
  trailingSlash: 'never',
});
```

**Step 4: Verify it builds**

```bash
cd site && npm run build
```
Expected: `dist/` directory created with no errors.

**Step 5: Commit**

```bash
git add site/
git commit -m "chore(site): initialize Astro project"
```

---

## Task 2: Copy and update public assets

**Files:**
- Create: `site/public/css/bbs.css` (from `docs/css/bbs.css`)
- Create: `site/public/js/bbs.js` (from `docs/js/bbs.js`)
- Create: `site/public/.nojekyll`
- Create: `site/public/ans/` (empty dir placeholder)

**Step 1: Copy assets**

```bash
mkdir -p site/public/css site/public/js site/public/ans
cp docs/css/bbs.css site/public/css/bbs.css
cp docs/js/bbs.js site/public/js/bbs.js
touch site/public/.nojekyll
```

**Step 2: Swap font in bbs.css**

Replace the `@import` line at the top of `site/public/css/bbs.css`:

Old:
```css
@import url('https://fonts.googleapis.com/css2?family=VT323&family=Share+Tech+Mono&display=swap');
```

New:
```css
@import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&display=swap');
```

Then replace ALL occurrences of font references throughout the file:
- `'Share Tech Mono', 'Courier New', monospace` → `'JetBrains Mono', 'Courier New', monospace`
- `'VT323', 'Share Tech Mono', monospace` → `'JetBrains Mono', 'Courier New', monospace`
- `'Share Tech Mono', monospace` → `'JetBrains Mono', monospace`
- `'VT323', monospace` → `'JetBrains Mono', monospace`

**Step 3: Commit**

```bash
git add site/public/
git commit -m "chore(site): copy assets, swap to JetBrains Mono"
```

---

## Task 3: Create Base and BBS layouts

**Files:**
- Create: `site/src/layouts/Base.astro`
- Create: `site/src/layouts/BBS.astro`

**Step 1: Create Base.astro**

```astro
---
export interface Props {
  title: string;
  description?: string;
}
const { title, description = 'orcai: AI workspace for hackers' } = Astro.props;
---
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>{title} — ORCAI BBS</title>
  <meta name="description" content={description} />
  <link rel="stylesheet" href="/orcai/css/bbs.css" />
</head>
<body>
  <slot />
  <script src="/orcai/js/bbs.js"></script>
</body>
</html>
```

**Step 2: Create BBS.astro**

```astro
---
import Base from './Base.astro';
import Nav from '../components/Nav.astro';
import Footer from '../components/Footer.astro';

export interface Props {
  title: string;
  description?: string;
  activePage?: string;
}
const { title, description, activePage = '' } = Astro.props;
---
<Base title={title} description={description}>
  <canvas id="hexbg"></canvas>
  <Nav activePage={activePage} />
  <main>
    <slot />
  </main>
  <Footer />
</Base>
```

**Step 3: Build check**

```bash
cd site && npm run build 2>&1 | tail -5
```
Expected: no errors.

**Step 4: Commit**

```bash
git add site/src/layouts/
git commit -m "feat(site): add Base and BBS layouts"
```

---

## Task 4: Create Nav and Footer components

**Files:**
- Create: `site/src/components/Nav.astro`
- Create: `site/src/components/Footer.astro`

**Step 1: Create Nav.astro**

```astro
---
export interface Props {
  activePage?: string;
}
const { activePage = '' } = Astro.props;

const links = [
  { href: '/orcai/', label: 'About', key: 'index' },
  { href: '/orcai/getting-started', label: 'Getting Started', key: 'getting-started' },
  { href: '/orcai/plugins', label: 'Plugins', key: 'plugins' },
  { href: '/orcai/pipelines', label: 'Pipelines', key: 'pipelines' },
  { href: '/orcai/themes', label: 'Themes', key: 'themes' },
  { href: '/orcai/changelog', label: 'Changelog', key: 'changelog' },
  { href: '/orcai/registry', label: 'Registry', key: 'registry' },
  { href: 'https://github.com/adam-stokes/orcai', label: 'GitHub', key: 'github', external: true },
];
---
<nav class="bbs-nav" aria-label="Main navigation">
  <div class="bbs-nav-inner">
    <a href="/orcai/" class={`nav-brand ${activePage === 'index' ? 'active' : ''}`}>ORCAI BBS</a>
    {links.map((link) => (
      <>
        <span class="sep">&nbsp;·&nbsp;</span>
        <a
          href={link.href}
          class={activePage === link.key ? 'active' : ''}
          {...(link.external ? { target: '_blank', rel: 'noopener' } : {})}
        >{link.label}</a>
      </>
    ))}
  </div>
  <div class="node-status">
    NODE-001 &nbsp;|&nbsp; <span class="online">● ONLINE</span> &nbsp;|&nbsp; <span class="clock"></span>
  </div>
</nav>
```

**Step 2: Create Footer.astro**

```astro
---
---
<footer class="bbs-footer">
  [ <a href="/orcai/">ORCAI BBS</a> ]
  &nbsp;[ EST. 2025 ]&nbsp;
  [ PRESS ESC TO QUIT ]&nbsp;
  [ <a href="https://opensource.org/licenses/MIT" target="_blank" rel="noopener">MIT LICENSE</a> ]
  <br />
  <span style="color:var(--selbg);font-size:10px;letter-spacing:1px">
    Built with Go · BubbleTea · tmux · Astro
  </span>
</footer>
```

**Step 3: Build check**

```bash
cd site && npm run build 2>&1 | tail -5
```

**Step 4: Commit**

```bash
git add site/src/components/Nav.astro site/src/components/Footer.astro
git commit -m "feat(site): add Nav and Footer components"
```

---

## Task 5: Create SysinfoBox and FeatureCard components

**Files:**
- Create: `site/src/components/SysinfoBox.astro`
- Create: `site/src/components/FeatureCard.astro`
- Create: `site/src/components/AnsiLogo.astro`

**Step 1: Create SysinfoBox.astro**

CSS-bordered table — no manual char padding:

```astro
---
---
<div class="sysinfo-box">
  <table class="sysinfo-table">
    <tbody>
      <tr>
        <td colspan="2" class="sysinfo-title">SYSTEM: ORCAI v0.1.0</td>
      </tr>
      <tr><td class="sysinfo-label">NODE:</td><td class="sysinfo-value">github.com/adam-stokes/orcai</td></tr>
      <tr><td class="sysinfo-label">OS:</td><td class="sysinfo-value">Linux · macOS · tmux required</td></tr>
      <tr><td class="sysinfo-label">LANG:</td><td class="sysinfo-value">Go 1.21+</td></tr>
    </tbody>
    <tbody class="sysinfo-menu">
      <tr>
        <td class="key-hint">[ENTER]</td>
        <td><a href="/orcai/getting-started" class="key-action">Getting Started</a></td>
      </tr>
      <tr>
        <td class="key-hint">[P]</td>
        <td><a href="/orcai/plugins" class="key-action">Plugin Gallery</a></td>
      </tr>
      <tr>
        <td class="key-hint">[R]</td>
        <td><a href="/orcai/registry" class="key-action">Plugin Registry</a></td>
      </tr>
      <tr>
        <td class="key-hint">[G]</td>
        <td><a href="https://github.com/adam-stokes/orcai" class="key-action" target="_blank" rel="noopener">GitHub</a></td>
      </tr>
    </tbody>
  </table>
</div>

<style>
.sysinfo-box {
  margin-top: 28px;
  display: inline-block;
}
.sysinfo-table {
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  line-height: 1.7;
  border-collapse: collapse;
  border: 1px solid var(--purple);
}
.sysinfo-table td {
  padding: 3px 16px;
}
.sysinfo-title {
  color: var(--purple);
  font-weight: bold;
  letter-spacing: 1px;
  border-bottom: 1px solid var(--selbg);
  padding: 6px 16px !important;
}
.sysinfo-label { color: var(--comment); white-space: nowrap; }
.sysinfo-value { color: var(--fg); }
.sysinfo-menu { border-top: 1px solid var(--selbg); }
.key-hint { color: var(--yellow); white-space: nowrap; }
.key-action { color: var(--fg); text-decoration: none; }
.key-action:hover { color: var(--cyan); }
</style>
```

**Step 2: Create FeatureCard.astro**

```astro
---
export interface Props {
  title: string;
  borderColor?: string;
}
const { title, borderColor = 'var(--purple)' } = Astro.props;
---
<div class="feature-card" style={`border-color: ${borderColor}`}>
  <h3>{title}</h3>
  <slot />
</div>
```

**Step 3: Create AnsiLogo.astro**

```astro
---
---
<pre class="ansi-logo" aria-label="ORCAI logo" id="ansi-logo">
 ██████╗ ██████╗  ██████╗ █████╗ ██╗
██╔═══██╗██╔══██╗██╔════╝██╔══██╗██║
██║   ██║██████╔╝██║     ███████║██║
██║   ██║██╔══██╗██║     ██╔══██╗██║
╚██████╔╝██║  ██╗╚██████╗██║  ██║██║
 ╚═════╝ ╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝╚═╝</pre>
```

Note: `bbs.js` `colorLogo()` targets `#ansi-logo` — keep the `id`.

**Step 4: Build check**

```bash
cd site && npm run build 2>&1 | tail -5
```

**Step 5: Commit**

```bash
git add site/src/components/
git commit -m "feat(site): add SysinfoBox, FeatureCard, AnsiLogo components"
```

---

## Task 6: Set up Content Collections

**Files:**
- Create: `site/src/content/config.ts`
- Create: `site/src/content/changelog/v0.1.0.md`
- Create: `site/src/content/registry/ripgrep.md`
- Create: `site/src/content/pipelines/01-quickstart.mdx`

**Step 1: Create content config**

```ts
// site/src/content/config.ts
import { defineCollection, z } from 'astro:content';

const changelog = defineCollection({
  type: 'content',
  schema: z.object({
    version: z.string(),
    date: z.string(),
  }),
});

const registry = defineCollection({
  type: 'content',
  schema: z.object({
    name: z.string(),
    description: z.string(),
    tier: z.union([z.literal(1), z.literal(2)]),
    repo: z.string().optional(),
    command: z.string().optional(),
    capabilities: z.array(z.string()),
  }),
});

const pipelines = defineCollection({
  type: 'content',
  schema: z.object({
    title: z.string(),
    description: z.string(),
    order: z.number(),
  }),
});

export const collections = { changelog, registry, pipelines };
```

**Step 2: Seed changelog entry**

```markdown
---
version: "0.1.0"
date: "2026-03-24"
---

## Initial public release

- BubbleTea TUI with tmux session management
- Plugin system: Tier 1 (gRPC) and Tier 2 (sidecar YAML)
- YAML prompt pipeline builder
- ANSI/BBS themed interface with Dracula palette
- Claude, Ollama, Copilot, OpenCode provider support
```

**Step 3: Seed registry entry**

```markdown
---
name: "ripgrep"
description: "Fast recursive search using rg"
tier: 2
command: "rg"
capabilities: ["search", "grep", "files"]
repo: "https://github.com/BurntSushi/ripgrep"
---

Wrap ripgrep as a Tier 2 orcai plugin via sidecar YAML.
Runs `rg --json` and pipes structured output into pipelines.
```

**Step 4: Seed pipeline reference**

```mdx
---
title: "Quickstart Pipeline"
description: "Your first orcai pipeline in 5 minutes"
order: 1
---

## Quickstart Pipeline

Create a file named `hello.pipeline.yaml`:

```yaml
name: hello
steps:
  - id: ask
    provider: claude
    prompt: "Say hello in the style of a 1990s BBS sysop"
```

Run it:

```bash
./bin/orcai pipeline run hello.pipeline.yaml
```
```

**Step 5: Build check**

```bash
cd site && npm run build 2>&1 | tail -5
```

**Step 6: Commit**

```bash
git add site/src/content/
git commit -m "feat(site): add Content Collections schema and seed content"
```

---

## Task 7: Build index page

**Files:**
- Create: `site/src/pages/index.astro`
- Delete: `site/src/pages/index.astro` (the default Astro one if it exists)

**Step 1: Create index.astro**

```astro
---
import BBS from '../layouts/BBS.astro';
import SysinfoBox from '../components/SysinfoBox.astro';
import FeatureCard from '../components/FeatureCard.astro';
import AnsiLogo from '../components/AnsiLogo.astro';
---
<BBS title="Home" activePage="index">
  <section class="hero" data-page="index">
    <div class="hero-subtitle fg-comment">// BBS NODE 001 // CONNECTED //</div>
    <AnsiLogo />
    <div class="typewriter-wrap">
      <span id="typewriter"></span>
    </div>
    <SysinfoBox />
    <p style="color:var(--comment);font-size:11px;margin-top:14px;letter-spacing:2px">
      PRESS <span style="color:var(--yellow)">ENTER</span> TO CONNECT &nbsp;·&nbsp;
      <span style="color:var(--yellow)">P</span> FOR PLUGINS &nbsp;·&nbsp;
      <span style="color:var(--yellow)">ESC</span> TO DISCONNECT
    </p>
  </section>

  <section id="about" class="content" style="position:relative;z-index:1">
    <h2 class="section-header">// What Is ORCAI //</h2>
    <p style="color:var(--fg);font-size:13px;line-height:2;max-width:680px;margin-bottom:8px">
      <span class="fg-purple">orcai</span> is a Go <span class="fg-cyan">BubbleTea</span> TUI
      that turns your terminal into an AI command center. Manage tmux sessions from a BBS-style
      sidebar, wire up any CLI tool as a plugin via gRPC or sidecar YAML, and run multi-step AI
      agent pipelines across Claude, Ollama, GitHub Copilot — all from one dark, dense, beautiful interface.
    </p>
    <div class="features">
      <FeatureCard title="TMUX SESSIONS" borderColor="var(--purple)">
        <p>Manage AI agent workspaces from a BBS-style sidebar TUI. Spawn, switch, and kill tmux sessions without leaving your workflow.</p>
      </FeatureCard>
      <FeatureCard title="PLUGIN SYSTEM" borderColor="var(--cyan)">
        <p>gRPC plugins + sidecar YAML. Any CLI tool becomes a pipeline node. Tier 1 native Go. Tier 2 wrappers for everything else.</p>
      </FeatureCard>
      <FeatureCard title="AGENT PIPELINES" borderColor="var(--green)">
        <p>YAML pipelines run Claude, Ollama, and Copilot at scale. Chain tools with typed inputs and outputs.</p>
      </FeatureCard>
    </div>
  </section>
</BBS>
```

**Step 2: Build and preview**

```bash
cd site && npm run build && npx serve dist -p 4000
```
Open `http://localhost:4000/orcai/` — verify hex canvas, logo, typewriter, sysinfo box all render.

**Step 3: Commit**

```bash
git add site/src/pages/index.astro
git commit -m "feat(site): migrate index page to Astro"
```

---

## Task 8: Migrate getting-started and plugins pages

**Files:**
- Create: `site/src/pages/getting-started.astro`
- Create: `site/src/pages/plugins.astro`

**Step 1: Create getting-started.astro**

Port content from `docs/getting-started.html`. Wrap in `<BBS title="Getting Started" activePage="getting-started">`. Keep all existing HTML structure inside, just strip the outer `<html>/<head>/<body>` and nav/footer (now in layout). Keep `<pre class="code-block">` blocks — copy buttons added by `bbs.js`.

**Step 2: Create plugins.astro**

Port content from `docs/plugins.html`. Same approach — wrap in `<BBS title="Plugins" activePage="plugins">`, strip outer shell, keep inner content.

**Step 3: Build check**

```bash
cd site && npm run build 2>&1 | grep -E "error|Error|warn" | head -10
```

**Step 4: Commit**

```bash
git add site/src/pages/getting-started.astro site/src/pages/plugins.astro
git commit -m "feat(site): migrate getting-started and plugins pages"
```

---

## Task 9: Build changelog and pipeline reference pages

**Files:**
- Create: `site/src/pages/changelog.astro`
- Create: `site/src/pages/pipelines.astro`

**Step 1: Create changelog.astro**

```astro
---
import BBS from '../layouts/BBS.astro';
import { getCollection } from 'astro:content';

const entries = await getCollection('changelog');
entries.sort((a, b) => b.data.version.localeCompare(a.data.version));
---
<BBS title="Changelog" activePage="changelog">
  <div class="content content-padded" style="position:relative;z-index:1">
    <h2 class="section-header">// Changelog //</h2>
    {entries.map(async (entry) => {
      const { Content } = await entry.render();
      return (
        <div class="changelog-entry">
          <h3 class="fg-purple">v{entry.data.version} <span class="fg-comment">— {entry.data.date}</span></h3>
          <div class="changelog-body">
            <Content />
          </div>
        </div>
      );
    })}
  </div>
</BBS>

<style>
.changelog-entry { margin-bottom: 40px; border-left: 2px solid var(--purple); padding-left: 20px; }
.changelog-entry h3 { font-family: 'JetBrains Mono', monospace; font-size: 16px; margin-bottom: 8px; }
.changelog-body { font-size: 13px; line-height: 1.8; color: var(--fg); }
.changelog-body ul { padding-left: 1.5em; }
.changelog-body li::marker { color: var(--cyan); }
</style>
```

**Step 2: Create pipelines.astro**

```astro
---
import BBS from '../layouts/BBS.astro';
import { getCollection } from 'astro:content';

const docs = await getCollection('pipelines');
docs.sort((a, b) => a.data.order - b.data.order);
---
<BBS title="Pipeline Reference" activePage="pipelines">
  <div class="content content-padded" style="position:relative;z-index:1">
    <h2 class="section-header">// Pipeline Reference //</h2>
    {docs.map(async (doc) => {
      const { Content } = await doc.render();
      return (
        <article class="pipeline-doc">
          <h3 class="fg-cyan">{doc.data.title}</h3>
          <p class="fg-comment" style="font-size:12px;margin-bottom:16px">{doc.data.description}</p>
          <Content />
        </article>
      );
    })}
  </div>
</BBS>

<style>
.pipeline-doc { margin-bottom: 48px; }
.pipeline-doc h3 { font-family: 'JetBrains Mono', monospace; font-size: 15px; margin-bottom: 4px; }
.pipeline-doc pre { background: var(--darkbg); border-left: 3px solid var(--cyan); padding: 12px 16px; overflow-x: auto; font-size: 13px; line-height: 1.6; position: relative; }
</style>
```

**Step 3: Build check**

```bash
cd site && npm run build 2>&1 | tail -5
```

**Step 4: Commit**

```bash
git add site/src/pages/changelog.astro site/src/pages/pipelines.astro
git commit -m "feat(site): add changelog and pipeline reference pages"
```

---

## Task 10: Build plugin registry pages

**Files:**
- Create: `site/src/pages/registry/index.astro`
- Create: `site/src/pages/registry/[slug].astro`

**Step 1: Create registry index**

```astro
---
import BBS from '../../layouts/BBS.astro';
import { getCollection } from 'astro:content';

const plugins = await getCollection('registry');
plugins.sort((a, b) => a.data.name.localeCompare(b.data.name));
---
<BBS title="Plugin Registry" activePage="registry">
  <div class="content content-padded" style="position:relative;z-index:1">
    <h2 class="section-header">// Plugin Registry //</h2>
    <p class="fg-comment" style="font-size:12px;margin-bottom:24px">
      Community plugins for orcai. <a href="https://github.com/adam-stokes/orcai/issues/new?template=plugin-submission.md">Submit yours →</a>
    </p>
    <div class="registry-grid">
      {plugins.map((plugin) => (
        <a href={`/orcai/registry/${plugin.slug}`} class="registry-card">
          <div class="registry-card-header">
            <span class="fg-purple">{plugin.data.name}</span>
            <span class={`tier-badge tier-${plugin.data.tier}`}>TIER {plugin.data.tier}</span>
          </div>
          <p>{plugin.data.description}</p>
          <div class="registry-caps">
            {plugin.data.capabilities.map(cap => <span class="cap-tag">{cap}</span>)}
          </div>
        </a>
      ))}
    </div>
  </div>
</BBS>

<style>
.registry-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.registry-card { border: 1px solid var(--selbg); padding: 14px 16px; text-decoration: none; color: var(--fg); font-size: 12px; line-height: 1.7; display: block; transition: border-color 0.15s; }
.registry-card:hover { border-color: var(--cyan); text-decoration: none; }
.registry-card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.tier-badge { font-size: 10px; letter-spacing: 1px; padding: 1px 6px; border: 1px solid; }
.tier-1 { color: var(--purple); border-color: var(--purple); }
.tier-2 { color: var(--comment); border-color: var(--comment); }
.registry-caps { margin-top: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
.cap-tag { font-size: 10px; color: var(--comment); border: 1px solid var(--selbg); padding: 0 5px; }
</style>
```

**Step 2: Create registry [slug] page**

```astro
---
import BBS from '../../layouts/BBS.astro';
import { getCollection, getEntry } from 'astro:content';

export async function getStaticPaths() {
  const plugins = await getCollection('registry');
  return plugins.map(p => ({ params: { slug: p.slug } }));
}

const { slug } = Astro.params;
const entry = await getEntry('registry', slug);
if (!entry) return Astro.redirect('/orcai/registry');
const { Content } = await entry.render();
---
<BBS title={entry.data.name} activePage="registry">
  <div class="content content-padded" style="position:relative;z-index:1">
    <p class="fg-comment" style="font-size:12px;margin-bottom:16px">
      <a href="/orcai/registry">← Registry</a>
    </p>
    <h2 class="section-header">// {entry.data.name} //</h2>
    <p style="font-size:13px;margin-bottom:20px;color:var(--fg)">{entry.data.description}</p>
    <table style="font-size:12px;border-collapse:collapse;margin-bottom:24px">
      <tr><td class="fg-comment" style="padding-right:24px;padding-bottom:4px">TIER</td><td class="fg-purple">{entry.data.tier}</td></tr>
      {entry.data.command && <tr><td class="fg-comment" style="padding-right:24px;padding-bottom:4px">COMMAND</td><td class="fg-green">{entry.data.command}</td></tr>}
      {entry.data.repo && <tr><td class="fg-comment" style="padding-right:24px;padding-bottom:4px">REPO</td><td><a href={entry.data.repo} target="_blank" rel="noopener">{entry.data.repo}</a></td></tr>}
    </table>
    <div class="plugin-content">
      <Content />
    </div>
  </div>
</BBS>

<style>
.plugin-content { font-size: 13px; line-height: 1.8; color: var(--fg); }
.plugin-content pre { background: var(--darkbg); border-left: 3px solid var(--green); padding: 12px 16px; overflow-x: auto; margin: 16px 0; }
</style>
```

**Step 3: Build check**

```bash
cd site && npm run build 2>&1 | tail -5
```

**Step 4: Commit**

```bash
git add site/src/pages/registry/
git commit -m "feat(site): add plugin registry pages"
```

---

## Task 11: Build themes / ANSI gallery page

**Files:**
- Create: `site/src/pages/themes.astro`

**Step 1: Create themes.astro**

```astro
---
import BBS from '../layouts/BBS.astro';
---
<BBS title="Themes" activePage="themes">
  <div class="content content-padded" style="position:relative;z-index:1">
    <h2 class="section-header">// ANSI Themes //</h2>
    <p class="fg-comment" style="font-size:12px;margin-bottom:8px">
      Custom ANSI art for orcai's welcome screen. Drop a <code>.ans</code> file into
      <code>~/.config/orcai/ui/welcome.ans</code> to override the default.
    </p>
    <p style="font-size:12px;margin-bottom:32px">
      <a href="https://github.com/adam-stokes/orcai/issues/new?template=theme-submission.md" target="_blank" rel="noopener">Submit a theme →</a>
    </p>

    <div class="theme-grid">
      <!-- Default theme -->
      <div class="theme-card">
        <div class="theme-preview">
<pre class="ansi-art" style="font-size:8px;line-height:1.1">
 ██████╗ ██████╗  ██████╗ █████╗ ██╗
██╔═══██╗██╔══██╗██╔════╝██╔══██╗██║
██║   ██║██████╔╝██║     ███████║██║
██║   ██║██╔══██╗██║     ██╔══██╗██║
╚██████╔╝██║  ██╗╚██████╗██║  ██║██║
 ╚═════╝ ╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝╚═╝</pre>
        </div>
        <div class="theme-meta">
          <span class="fg-purple">default</span>
          <span class="fg-comment">by orcai team</span>
        </div>
      </div>

      <!-- Submit placeholder -->
      <a href="https://github.com/adam-stokes/orcai/issues/new?template=theme-submission.md"
         class="theme-card theme-submit" target="_blank" rel="noopener">
        <div class="theme-preview theme-preview-empty">
          <span class="fg-comment">+ SUBMIT THEME</span>
        </div>
        <div class="theme-meta">
          <span class="fg-comment">your art here</span>
        </div>
      </a>
    </div>

    <h3 class="section-header" style="margin-top:48px;font-size:18px">// How To Install //</h3>
    <pre class="code-block"># Copy any .ans file to override the default welcome screen
mkdir -p ~/.config/orcai/ui/
cp mytheme.ans ~/.config/orcai/ui/welcome.ans
# Restart orcai to see changes</pre>
  </div>
</BBS>

<style>
.theme-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 16px; }
.theme-card { border: 1px solid var(--selbg); padding: 0; overflow: hidden; text-decoration: none; display: block; transition: border-color 0.15s; }
.theme-card:hover { border-color: var(--purple); }
.theme-preview { background: var(--darkbg); padding: 16px; min-height: 100px; display: flex; align-items: center; justify-content: center; }
.theme-preview-empty { border: 1px dashed var(--selbg); margin: 8px; font-size: 13px; letter-spacing: 2px; }
.theme-meta { padding: 8px 12px; display: flex; justify-content: space-between; font-size: 11px; border-top: 1px solid var(--selbg); }
</style>
```

**Step 2: Build check**

```bash
cd site && npm run build 2>&1 | tail -5
```

**Step 3: Commit**

```bash
git add site/src/pages/themes.astro
git commit -m "feat(site): add ANSI themes gallery page"
```

---

## Task 12: Update GitHub Actions workflow

**Files:**
- Modify: `.github/workflows/gh-pages.yml`

**Step 1: Replace workflow content**

```yaml
name: Deploy GitHub Pages

on:
  push:
    branches: [main]
    paths:
      - 'site/**'
      - 'assets/ui/**'
      - '.github/workflows/gh-pages.yml'
  workflow_dispatch:

permissions:
  contents: write
  pages: write
  id-token: write

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
          cache-dependency-path: site/package-lock.json

      - name: Install dependencies
        run: cd site && npm ci

      - name: Build Astro site
        run: cd site && npm run build

      - name: Deploy to GitHub Pages
        uses: peaceiris/actions-gh-pages@v4
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./site/dist
          publish_branch: gh-pages
          user_name: 'github-actions[bot]'
          user_email: 'github-actions[bot]@users.noreply.github.com'
          commit_message: 'deploy: ${{ github.sha }}'
```

**Step 2: Commit and push**

```bash
git add .github/workflows/gh-pages.yml
git commit -m "chore(ci): update gh-pages workflow for Astro build"
git push origin main
```

**Step 3: Watch the workflow**

```bash
gh run watch --repo adam-stokes/orcai $(gh run list --repo adam-stokes/orcai --limit 1 --json databaseId -q '.[0].databaseId')
```
Expected: all steps green.

---

## Task 13: Verify live site and clean up

**Step 1: Check live site**

```bash
curl -s -o /dev/null -w "%{http_code}" https://adam-stokes.github.io/orcai/
```
Expected: `200`

Check all routes:
```bash
for path in "" "getting-started" "plugins" "pipelines" "changelog" "registry" "themes"; do
  code=$(curl -s -o /dev/null -w "%{http_code}" "https://adam-stokes.github.io/orcai/$path")
  echo "$code  /orcai/$path"
done
```
Expected: all `200`.

**Step 2: Remove old HTML files from docs/**

```bash
rm docs/index.html docs/getting-started.html docs/plugins.html docs/_config.yml docs/.nojekyll
```

Keep `docs/plans/` and `docs/css/`, `docs/js/` as historical artifacts (gitignored in next step).

**Step 3: Commit cleanup**

```bash
git add -A
git commit -m "chore(site): remove legacy HTML files, Astro is now source of truth"
git push origin main
```

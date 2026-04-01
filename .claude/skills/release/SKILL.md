---
name: release
description: Cut a new GLITCH release via PR-protected main flow: branch → test → PR → merge → changelog curation → semver tag → GitHub Actions release build.
disable-model-invocation: true
---

Cut a new GLITCH release following the PR-before-tag flow. Never push a semver tag before merging to `main` via PR.

---

## Step 1 — Prerequisites check

Run the following and report results to the developer:

```bash
# Verify working tree is clean
git status --porcelain
```

If the output is non-empty, stop and tell the developer: "Working tree is dirty. Commit or stash all changes before releasing."

```bash
# Verify main is up-to-date with origin
git fetch origin main
git rev-list HEAD..origin/main --count
```

If the count is non-zero, stop and tell the developer: "Local main is behind origin/main. Run `git pull origin main` first."

```bash
# Report latest tag
git describe --tags --abbrev=0 2>/dev/null || echo "none"
```

Print: "Current latest tag: {TAG}"

---

## Step 2 — Determine next version

Parse the latest tag as semver `vMAJOR.MINOR.PATCH`. If no tag exists, treat current as `v0.0.0`.

Ask the developer: "Choose bump type: major / minor / patch"

Calculate the next version:
- **major**: increment MAJOR, reset MINOR and PATCH to 0
- **minor**: increment MINOR, reset PATCH to 0
- **patch**: increment PATCH only

Ask the developer: "Release v{VERSION}? (y/n)"

If the answer is not `y`, abort.

---

## Step 3 — Create release branch and PR

```bash
git checkout -b release/v{VERSION}
```

Run tests:

```bash
task test
```

If `task test` fails, stop and tell the developer: "Tests failed. Fix failures before releasing, then re-run `/release`."

Push the branch and open a PR:

```bash
git push -u origin release/v{VERSION}
gh pr create --title "chore: release v{VERSION}" --body "Release v{VERSION}"
```

Print the PR URL. Tell the developer: "PR created. Wait for all CI checks to pass before proceeding to Step 4."

---

## Step 4 — Wait for CI and merge

Tell the developer:

> Wait for all GitHub Actions checks to pass on the PR. Once green, merge with:
>
> ```bash
> gh pr merge --squash --delete-branch
> ```
>
> Or merge via the GitHub UI. Then press enter to continue.

Wait for the developer to confirm the PR is merged, then:

```bash
git checkout main
git pull origin main
```

Confirm the merge commit is present. If `main` has not advanced, stop and tell the developer: "It looks like the PR may not be merged yet. Merge it first, then continue."

---

## Step 5 — Generate and curate changelog

**Important:** The `highlights` array in the changelog file becomes the GitHub Release body. Write it for an audience of hackers and AI enthusiasts — lead with capabilities, not implementation noise.

Collect commits since last tag:

```bash
git log $(git describe --tags --abbrev=0 2>/dev/null)..HEAD --oneline --no-merges | grep -E '^[a-f0-9]+ (feat|fix|perf|refactor)'
```

Create `site/src/content/changelog/v{VERSION}.md`:

```markdown
---
version: "{VERSION}"
date: "{TODAY_DATE}"
highlights:
  - "feat: <capability> — <one-line description of what it enables>"
  - "feat: <capability> — <one-line description>"
  - "fix: <what was broken and how it manifests>"
breaking: false
---

## v{VERSION} — {TODAY_DATE}

<Optional 1-2 sentence summary for the changelog page.>
```

Rules for highlights:
- Each line is one user-visible capability or fix — no internal refactors, no CI/chore commits
- `feat:` lines describe new power the user has; `fix:` lines describe something that was broken
- Set `breaking: true` if any public interface, config format, or behaviour changed incompatibly
- Aim for 5–10 highlights — enough to tell the story, not so many it becomes noise

Tell the developer:

> Review `site/src/content/changelog/v{VERSION}.md`. These highlights will appear on the GitHub Release page and the GLITCH website changelog. Press enter when ready to commit and tag.

Wait for the developer to confirm.

---

## Step 6 — Commit changelog and push tag

```bash
git add site/src/content/changelog/v{VERSION}.md
git commit -m "docs: add changelog for v{VERSION}"
git push origin main
```

Create a signed annotated tag and push it to trigger the release workflow:

```bash
git tag -s v{VERSION} -m "Release v{VERSION}"
git push origin v{VERSION}
```

Print: "Tag v{VERSION} pushed. GitHub Actions release workflow is now running."

---

## Step 7 — Monitor release

Print:

> Release v{VERSION} is building.
>
> Monitor GitHub Actions: https://github.com/8op-org/gl1tch/actions
>
> Once complete, the GitHub Release page will have all platform binaries and checksums.txt.
>
> The site changelog will auto-deploy via `gh-pages.yml` once the changelog commit lands on main.

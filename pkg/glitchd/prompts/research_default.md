# Research prompt

This file tells gl1tch what counts as "needs my attention" in this
workspace and what it should do when such an event lands. It is read
by the local attention classifier and by the deep-analysis prompt.

Edit it freely. Everything below is markdown the local LLM reads as
instructions — there is no schema, no parser. Be concrete.

## Who I am

The attention classifier looks up `git config user.name` and
`git config user.email` automatically. If you have additional handles
(GitHub login, GitLab login, Slack handle, display names on reviews)
list them here so the classifier can match them in author fields and
mentions:

- (add your handles here, e.g. `github: @adam-stokes`)

## What counts as high attention

Flag an event as `high` attention when ANY of the following is true.
Be honest — over-flagging is worse than under-flagging because a
high-attention event interrupts me.

- A review, review comment, or issue comment landed on a PR or issue
  I authored.
- A failing CI check, build, or test run landed on a branch I am
  working on.
- Someone mentioned me directly (`@me`) or assigned me to a PR/issue.
- A commit landed on `main`/`master`/`trunk` of a repository I own or
  actively contribute to, from an author other than me, that touches
  code I have recently changed.
- A security-shaped signal: leaked credential, force-push to a
  protected branch, permission change, new admin.

Flag as `low` when the event is clearly noise (dependabot, release
tags, merge commits from me, my own commits coming back through the
PR mirror). Everything else is `normal`.

## What to produce for high-attention events

When the deep analyzer runs on a high-attention event, it should NOT
stop at a summary. It should draft the artifact I would need to act on
the event, using its shell tools to gather full context first. The
artifact depends on the event shape — use judgment:

- **Review comment on my PR** — draft a reply that addresses each
  reviewer point inline, quotes the line of code being discussed, and
  ends with a one-sentence summary of what I will change. If the
  reviewer is asking for a code change, also produce the patch as a
  fenced diff block.
- **Failing CI check on my branch** — read the failing job's log,
  identify the failing test / assertion / step, and draft either the
  fix (as a diff) or a concrete next command I should run locally to
  reproduce.
- **Mention / assignment** — draft the reply I would send, grounded in
  the thread history and any referenced code.
- **Commit on main touching my code** — summarize what changed in my
  area, whether it conflicts with anything I have in flight, and draft
  the rebase / merge command I would run.

For every high-attention artifact, include a one-line TL;DR at the
top, then the artifact itself, then a short "how I got here" note
listing the shell commands you actually ran to gather context.

## Escalation

Set `escalate: on-request` if you only want the paid polish step
(step 5) to run when I explicitly click Escalate on a card. Set
`escalate: auto-high` to automatically run the polish step on every
high-attention event. Default is on-request.

escalate: on-request

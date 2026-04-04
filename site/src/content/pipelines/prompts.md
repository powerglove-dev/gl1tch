---
title: "Prompts"
description: "Build and save reusable instructions — tell gl1tch what you want once, reuse it anywhere."
order: 99
---

Some instructions you write once and never want to repeat. Your code review voice. Your commit message style. Your preferred debugging approach. Prompts let you save those instructions and reload them in any conversation — just ask gl1tch to build one.


## Quick Start

In the gl1tch chat panel:

```
/prompt code-reviewer Review for correctness, clarity, and edge cases. Be concise and direct.
```

gl1tch generates a full prompt from your description, shows you the result, and saves it as `code-reviewer`. Load it later with:

```
/prompt code-reviewer
```


## Creating a Prompt

The `/prompt` command has two forms.

**Inline — name and description in one line:**

```
/prompt <name> <description>
```

gl1tch generates a prompt from your description and saves it immediately.

**Guided — describe in a follow-up:**

```
/prompt
```

gl1tch asks what the prompt should do. Describe it in the next message. gl1tch generates the prompt, shows it to you, then asks what to name it.

Saved prompts are stored at `~/.config/glitch/prompts/<name>.md`.


## Loading a Saved Prompt

```
/prompt <name>
```

gl1tch displays the full prompt text in the conversation. Use this to review it or copy it into a pipeline step.


## Examples

**Code reviewer:**
```
/prompt reviewer Give direct, actionable feedback on correctness and edge cases. Skip praise.
```

**Commit message style:**
```
/prompt commits Write conventional commit messages. Use feat/fix/chore/docs. Keep the subject under 72 characters.
```

**Debug helper:**
```
/prompt debugger Walk through the problem systematically. State your hypothesis before suggesting a fix.
```


## See Also

- [Pipelines](/docs/pipelines/pipelines) — automate multi-step work
- [Brain](/docs/pipelines/brain) — gl1tch remembering context across sessions

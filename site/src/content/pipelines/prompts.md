---
title: "Prompts"
description: "Build and save reusable instructions — tell gl1tch what you want once, reuse it anywhere."
order: 99
---

Some instructions you write once and never want to repeat. Your code review voice. Your commit message style. Your preferred debugging approach. Prompts let you save those instructions and reload them in any conversation — just ask gl1tch to build one.


## Quick Start

In the gl1tch chat panel:

```bash
/prompt code-reviewer Review for correctness, clarity, and edge cases. Be concise and direct.
```

gl1tch generates a full prompt from your description, shows you the result, and saves it as `code-reviewer`. Load it later with:

```bash
/prompt code-reviewer
```


## Creating a Prompt

The `/prompt` command has two forms.

**Inline — name and description in one line:**

```bash
/prompt <name> <description>
```

gl1tch generates a prompt from your description and saves it immediately.

**Guided — describe in a follow-up:**

```bash
/prompt
```

gl1tch asks what the prompt should do. Describe it in the next message. gl1tch generates the prompt, shows it to you, then asks what to name it.

Saved prompts are stored in your profile and persist across sessions.


## Using a Prompt

Load a saved prompt by name:

```bash
/prompt <name>
```

gl1tch displays the full prompt text in the conversation.

**List all your prompts:**

```bash
/prompt list
```

**Edit an existing prompt:**

```bash
/prompt <name> --edit
```

**Delete a prompt:**

```bash
/prompt <name> --delete
```


## Using Prompts in Pipelines

Inject a saved prompt into any pipeline step by adding the `prompt:` field:

```yaml
steps:
  - name: review-code
    run: github
    prompt: code-reviewer
```

When this step runs, gl1tch prepends your saved prompt to the step's input before execution. This lets you enforce your voice across multiple pipelines without rewriting the same instructions.


## Customizing

**Iterate on a prompt:** Use the chat to test a prompt, refine it based on results, then save the updated version:

```bash
/prompt code-reviewer My updated instructions go here.
```

gl1tch overwrites the previous version.

**Cross-project reuse:** Your prompts are global — save them once, use them in any pipeline or conversation, whether local or remote.

**Test before production:** Use `/prompt <name>` to load and review a prompt in the chat before wiring it into a pipeline step.


## Examples

**Code reviewer:**

```bash
/prompt reviewer Give direct, actionable feedback on correctness and edge cases. Skip praise.
```

**Commit message guide:**

```bash
/prompt commit-guide Write conventional commits. Format: type(scope): short summary. Type is feat, fix, chore, or docs.
```

**Security audit:**

```bash
/prompt security-audit Check for injection vulnerabilities, missing input validation, hardcoded secrets, and unsafe deserialization.
```

**Documentation writer:**

```bash
/prompt docs-writer Explain for someone new to the codebase. Start with why, then how. Use examples.
```

**Debugging partner:**

```bash
/prompt debug Ask clarifying questions before suggesting fixes. Focus on root cause, not symptoms.
```


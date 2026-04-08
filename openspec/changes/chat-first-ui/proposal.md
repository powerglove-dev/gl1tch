## Why

Today the desktop UI has a left sidebar carrying navigation, ambient state, and configuration entry points, plus a chat window. Two competing metaphors. For a daily driver the chat metaphor wins — every affordance becomes addressable by typing or by clicking a widget the assistant just rendered. The sidebar costs screen real estate, splits attention, and forces every new feature to choose between "is this a sidebar item or a chat capability?" Killing it forces the right answer (it's a chat capability) and lets us put the attention feed, brain config, evidence bundles, and next-step prompts where the user is already looking.

## What Changes

- Remove the left sidebar from the desktop UI. **BREAKING** for any user muscle memory tied to sidebar buttons.
- Keep the **workspace switcher** as a thin top strip above chat (the only persistent navigation surface).
- Introduce **rich chat message types**: chat is no longer markdown-only. New types include `widget_card`, `action_chips`, `evidence_bundle`, `score_card`, `attention_feed`. Each is a structured payload that the chat renderer paints natively.
- The **attention feed** (already shipped per `bcf1eb4`) becomes the *default* content of an idle chat session — when there's no live conversation, the chat panel shows the current attention items as `attention_feed` messages. The moment the user types, the feed compacts to a one-line "N items need you" strip pinned at the top.
- Add **slash commands** (`/status`, `/sessions`, `/brain config`, `/researcher list`, `/help`) as the keyboard path for everything that used to live in the sidebar. Slash commands return widget messages, not plain text, where appropriate.
- Add a **widget action protocol**: widget cards can declare clickable buttons (`action_chips`) whose clicks send a structured message back through the chat input, equivalent to the user typing the corresponding slash command. No new event bus.
- Move **brain configuration**, **researcher list**, **session history**, and **status/health** out of the sidebar and into widget cards spawned by their slash commands.
- Render the **evidence bundle and confidence score** from `glitch-research-loop` as a native `evidence_bundle` widget under each assistant answer, with the per-signal breakdown collapsed by default.

## Capabilities

### New Capabilities
- `chat-message-types`: the structured message-type protocol that lets chat carry non-markdown payloads (`widget_card`, `action_chips`, `evidence_bundle`, `score_card`, `attention_feed`).
- `chat-slash-commands`: the slash-command surface and dispatcher that replaces sidebar navigation with typed commands.
- `attention-feed-idle-default`: the rule that when a chat session is idle the attention feed occupies the panel, and that it compacts to a pinned strip the moment the user starts typing.
- `widget-action-protocol`: how widget buttons round-trip back into chat input as structured messages without a separate event bus.

### Modified Capabilities
_None at the spec level — this is a UI restructure that builds on existing chat and attention infrastructure. No prior capability has a sidebar requirement to remove (the sidebar was an implementation choice, not a spec requirement)._

## Impact

- **Touched packages**: `internal/chatui` (new message types, renderer changes, slash dispatcher), `cmd/glitch-desktop` or wherever the desktop layout lives (sidebar removal, workspace switcher relocation), `internal/activity` (attention feed adapter to the new message type).
- **Visual breaking change**: users will notice the sidebar is gone on first launch. Mitigated by a one-time welcome message in chat explaining the new layout and the slash commands.
- **No backend breakage**: every removed sidebar feature has a slash-command equivalent. Nothing is lost, only relocated.
- **Out of scope**: redesigning the chat input area, changing the color theme, adding new widget types beyond the five named above, animations or transitions.
- **Non-goals**: bringing the sidebar back behind a flag (per the no-migrations rule, we wipe and restart); supporting a "classic mode"; adding an icon-only sidebar; supporting drag-and-drop widget rearrangement.
- **Depends on**: `glitch-research-loop` for the `evidence_bundle` widget content, but the message type itself is decoupled — it can ship empty if the loop isn't ready.

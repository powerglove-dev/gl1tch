## ADDED Requirements

### Requirement: Discriminated chat message types
The chat data model SHALL represent each message as a discriminated union with a `Type` field and a type-specific `Payload`. Supported types in v1 are: `text`, `widget_card`, `action_chips`, `evidence_bundle`, `score_card`, `attention_feed`.

#### Scenario: Renderer dispatches on Type
- **WHEN** the chat renderer encounters a message with `Type = widget_card`
- **THEN** it SHALL invoke the widget-card renderer with the structured payload and SHALL NOT attempt to parse it as markdown

#### Scenario: Unknown type renders as fallback text
- **WHEN** a chat session is loaded that contains a message with a `Type` the renderer does not recognize
- **THEN** the renderer SHALL display a fallback "[unsupported widget]" placeholder and SHALL NOT crash

### Requirement: ParentID field for forward-compatible threading
Every chat message SHALL carry a nullable `ParentID` field referencing the message that started its containing thread, where `nil` denotes a top-level message in the main chat. The renderer SHALL use this field to group messages by thread once threading is implemented; in v1 the field is always `nil` and is reserved purely for forward-compatibility with the planned `chat-threads` change.

#### Scenario: Top-level message has nil ParentID
- **WHEN** a message is created from main-chat input or a slash command
- **THEN** its `ParentID` SHALL be `nil` and the renderer SHALL display it in the main chat stream

#### Scenario: Persistence round-trips ParentID
- **WHEN** a message with any `ParentID` value is serialized and reloaded
- **THEN** the reloaded message SHALL have the same `ParentID` value, including when that value is `nil`

#### Scenario: Reply count affordance is rendered when present
- **WHEN** the chat history contains one or more messages whose `ParentID` points at a given parent message
- **THEN** the parent message SHALL display a compact reply-count affordance (e.g. "💬 N replies"); clicking the affordance is a no-op in v1 and is wired by the `chat-threads` change

### Requirement: widget_card payload
A `widget_card` payload SHALL contain a title, an optional subtitle, an ordered list of key/value rows, and an optional list of action buttons.

#### Scenario: Brain config card renders
- **WHEN** `/brain config` returns a `widget_card` with title "Brain Configuration" and threshold/budget rows
- **THEN** the chat panel SHALL render a card with the title, the rows as a key/value table, and any declared action buttons

### Requirement: action_chips payload
An `action_chips` payload SHALL contain an ordered list of clickable chips, each with a label and an `action` string.

#### Scenario: Suggested next prompts render under an answer
- **WHEN** an assistant message is followed by an `action_chips` message containing three chips
- **THEN** the chat panel SHALL render the chips as inline buttons under the answer, in order

### Requirement: evidence_bundle payload
An `evidence_bundle` payload SHALL contain the list of researcher names invoked, the per-claim grounding labels, the per-signal score breakdown, and the final composite confidence.

#### Scenario: Evidence bundle from research loop renders collapsed
- **WHEN** an assistant message includes an `evidence_bundle` payload with five claims and four signals
- **THEN** the chat panel SHALL render a compact summary (researcher count + composite confidence) with an expand affordance that reveals the full breakdown

### Requirement: score_card payload
A `score_card` payload SHALL contain a metric name, a current value, and an optional historical series for sparkline rendering.

#### Scenario: Brain accept-rate score card renders
- **WHEN** `/brain stats` returns a `score_card` with metric "accept_rate" and the last 30 days of values
- **THEN** the chat panel SHALL render the metric name, the current value, and a sparkline of the historical series

### Requirement: attention_feed payload
An `attention_feed` payload SHALL contain an ordered list of attention items with title, source, confidence, and an optional `action` per item.

#### Scenario: Idle chat renders the attention feed
- **WHEN** a chat session is idle and the activity package emits attention items
- **THEN** the chat panel SHALL render an `attention_feed` message at the top of the visible area listing those items

### Requirement: Persistence of widget messages
Chat history serialization SHALL include the `Type` and `Payload` of every message so that reloading a session repaints widgets in their last visible state.

#### Scenario: Reloaded session repaints a widget card
- **WHEN** a chat session containing a `widget_card` message is closed and reloaded
- **THEN** the same widget card SHALL appear in the same position with the same content

#### Scenario: Persisted action buttons are inert
- **WHEN** a reloaded session displays a widget card from a past session that contained action buttons
- **THEN** the buttons SHALL be rendered as inert (visibly disabled) so clicks do not replay past actions

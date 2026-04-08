## ADDED Requirements

### Requirement: Reply-count affordance becomes interactive
The reply-count affordance defined by `chat-first-ui` SHALL become interactive: clicking it SHALL toggle the thread's expand state between collapsed and inline-expanded.

#### Scenario: Click expands the thread inline
- **WHEN** the user clicks a reply-count affordance on a parent message whose thread is currently collapsed
- **THEN** the thread's messages SHALL render inline directly below the parent message in the main chat stream

#### Scenario: Second click collapses the thread
- **WHEN** the user clicks the affordance again
- **THEN** the inline thread view SHALL collapse and the parent SHALL show only the reply-count affordance again

### Requirement: Inline expand is bounded in height
An inline-expanded thread SHALL render at most 5 messages of visible height, with internal vertical scrolling for longer threads.

#### Scenario: Long thread scrolls internally
- **WHEN** an inline-expanded thread contains more than 5 messages
- **THEN** the visible portion SHALL show 5 messages and the user SHALL be able to scroll within the thread without affecting the main chat scroll position

### Requirement: Side-pane mode
Threads SHALL support a side-pane display mode where the thread occupies a right-hand pane next to the main chat. The mode SHALL be toggled via a thread action button or by holding shift while clicking the affordance.

#### Scenario: Shift-click opens side pane
- **WHEN** the user shift-clicks the reply-count affordance
- **THEN** the thread SHALL open in a side pane occupying the right third of the chat panel and the main chat SHALL scroll independently

#### Scenario: Side-pane preference is per-thread
- **WHEN** a thread is opened in side-pane mode and then closed and re-opened
- **THEN** it SHALL re-open in side-pane mode unless the user explicitly switched it back to inline

### Requirement: Closed threads are read-only in the renderer
A thread in `closed` state SHALL render with disabled input and disabled widget action buttons, matching the inert-past-session affordance from `chat-first-ui`.

#### Scenario: Closed thread input is disabled
- **WHEN** the user expands a closed thread
- **THEN** the thread's input field SHALL be visibly disabled and any widget action buttons SHALL be inert

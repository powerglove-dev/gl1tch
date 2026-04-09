// ThreadSidePane.tsx is the Slack-style right-side panel that opens
// when the user clicks the 💬 affordance on any message in the chat.
// It is the canonical thread surface for gl1tch — the chat itself is
// the threaded experience, and this pane is where each thread lives.
//
// Responsibilities:
//   - Fetch the thread's messages on mount and after every dispatch.
//   - Render every chatui message type (text, widget_card, action_chips,
//     evidence_bundle, score_card) so a thread can carry rich content.
//   - Send the user's input through DispatchSlash with thread scope, so
//     freeform text and `/research` both produce a research-grounded
//     reply that lands in this thread.
//   - Drill into evidence rows by spawning a sibling thread under the
//     same parent message and swapping the pane to it (no nesting).
//   - Freeze (close) and reopen the thread.
import type { ReactElement } from "react";
import { useEffect, useState, useCallback } from "react";
import {
  DispatchSlash,
  GetThreadMessages,
  SpawnDrillThreadFromEvidence,
} from "../../wailsjs/go/main/App";
import { BlockRenderer } from "./MessageList";
import type { Message } from "@/lib/types";

type Role = "user" | "assistant" | "system";
type MessageType =
  | "text"
  | "widget_card"
  | "action_chips"
  | "evidence_bundle"
  | "score_card"
  | "attention_feed";

interface ChatMessage {
  id: string;
  parent_message_id?: string;
  thread_id?: string;
  role: Role;
  type: MessageType;
  payload: any;
  created_at: string;
}

interface EvidenceBundleItem {
  source: string;
  title: string;
  body: string;
  refs?: string[];
}

interface Props {
  workspaceID: string;
  threadID: string;
  parentMessageID: string;
  /** The full chat-store Message the thread was spawned on. Rendered
   *  at the top of the side pane using the same BlockRenderer the
   *  main chat uses, so the thread starts with full context (the
   *  attention card, the gh CLI snippets, the brain note — whatever
   *  the parent actually contains) instead of a truncated preview. */
  parentMessage?: Message;
  /** Action handler forwarded into the parent's blocks. Forwards to
   *  the App's existing handleAction so action chips inside the
   *  parent message keep working from inside a thread. */
  onAction: (method: string, args?: unknown[]) => Promise<void>;
  /** Render-prop for the input bar. The host (App.tsx) passes a
   *  function that returns the same <ChatInput> the main chat uses,
   *  with all the same provider/model/chain/workflow controls — but
   *  whose onSend forwards into the thread's dispatch path. The pane
   *  exposes (dispatchInThread, busy) so the render function can
   *  wire onSend and disable controls during in-flight calls. */
  renderInput: (
    dispatchInThread: (text: string) => Promise<void>,
    busy: boolean,
  ) => ReactElement;
  /** Hide the pane. */
  onClose: () => void;
  /** Switch the pane to a different thread (used by evidence drill). */
  onSwitchThread: (threadID: string) => void;
}

// localPendingMessage is the optimistic-update placeholder we render
// for the user's typed line between submit and the backend reply. It
// disappears as soon as refresh() pulls the persisted version. The
// pending state also drives a "thinking…" pill so the user can see the
// research loop is working without staring at an empty pane.
type LocalPending = {
  body: string;
  startedAt: number;
} | null;

export function ThreadSidePane({
  workspaceID,
  threadID,
  parentMessageID,
  parentMessage,
  onAction,
  renderInput,
  onClose,
  onSwitchThread,
}: Props) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [busy, setBusy] = useState(false);
  // pending is the optimistic placeholder for the in-flight user
  // turn. We render it immediately so the user sees their own line
  // before the loop starts thinking; refresh() drops it once the
  // persisted version arrives from the backend.
  const [pending, setPending] = useState<LocalPending>(null);

  const refresh = useCallback(async () => {
    // Empty threadID means the host (App.tsx) opened the pane
    // optimistically and the SpawnThreadOnMessage round-trip is
    // still in flight. Skip the fetch — the parent message renders
    // immediately, and we'll auto-refresh once the real id arrives.
    if (!workspaceID || !threadID) return;
    try {
      const json = await GetThreadMessages(workspaceID, threadID);
      setMessages(JSON.parse(json) || []);
    } catch (err) {
      console.error("ThreadSidePane: GetThreadMessages failed", err);
    }
  }, [workspaceID, threadID]);

  useEffect(() => {
    // Drop any stale pending state when we switch threads or when
    // the optimistic placeholder resolves to a real id.
    setPending(null);
    setMessages([]);
    void refresh();
  }, [refresh]);

  // dispatchInThread is the thread-scoped dispatch the host's
  // <ChatInput> calls into via the renderInput prop. It owns the
  // optimistic-update path so the side pane stays responsive
  // regardless of which input component the host wires up.
  //
  // No-op while threadID is empty: the optimistic open path means
  // the user can theoretically type a follow-up before the spawn
  // round-trip resolves. We could queue the message, but the more
  // honest thing is to disable the input — handled in the render
  // prop via the `busy` flag we expose. We early-return here as a
  // belt-and-braces guard.
  const dispatchInThread = useCallback(
    async (text: string) => {
      const line = text.trim();
      if (!line || !threadID) return;
      setPending({ body: line, startedAt: Date.now() });
      setBusy(true);
      try {
        await DispatchSlash(workspaceID, line, `thread:${threadID}`);
      } catch (err) {
        console.error("ThreadSidePane: dispatch failed", err);
      } finally {
        setBusy(false);
        setPending(null);
        await refresh();
      }
    },
    [workspaceID, threadID, refresh],
  );

  // The header eyebrow is just "thread" — the parent message itself
  // is rendered at the top of the messages list, so the user sees
  // the actual context (full blocks, attention cards, code, refs)
  // instead of a one-line truncation. The pane no longer needs a
  // separate textual title.

  const drillIntoEvidence = useCallback(
    async (sourceMessageID: string, item: EvidenceBundleItem) => {
      try {
        const threadJSON = await SpawnDrillThreadFromEvidence(
          workspaceID,
          parentMessageID,
          JSON.stringify(item),
        );
        const next = JSON.parse(threadJSON);
        if (next && next.id) {
          onSwitchThread(next.id);
        }
        // Suppress an unused-var warning on the source message ID;
        // we keep the parameter so a future "drill from inside this
        // message specifically" affordance has it.
        void sourceMessageID;
      } catch (err) {
        console.error("ThreadSidePane: drill failed", err);
      }
    },
    [workspaceID, parentMessageID, onSwitchThread],
  );

  // ── payload renderers ────────────────────────────────────────────────────

  const renderPayload = (msg: ChatMessage): ReactElement => {
    switch (msg.type) {
      case "text":
        return <div className="threaded-chat-text">{msg.payload?.body || ""}</div>;
      case "widget_card":
        return (
          <div className="threaded-chat-card">
            <div className="threaded-chat-card-title">{msg.payload?.title}</div>
            {msg.payload?.subtitle && (
              <div className="threaded-chat-card-subtitle">{msg.payload.subtitle}</div>
            )}
            {(msg.payload?.rows || []).map(
              (row: { key: string; value: string }, idx: number) => (
                <div className="threaded-chat-card-row" key={idx}>
                  <code className="threaded-chat-card-key">{row.key}</code>
                  <span className="threaded-chat-card-value">{row.value}</span>
                </div>
              ),
            )}
          </div>
        );
      case "evidence_bundle":
        return (
          <div className="threaded-chat-bundle">
            <div className="threaded-chat-bundle-header">
              evidence ({(msg.payload?.items || []).length})
              {msg.payload?.composite ? (
                <span className="threaded-chat-bundle-confidence">
                  · confidence {msg.payload.composite.toFixed(2)}
                </span>
              ) : null}
            </div>
            {(msg.payload?.items || []).map((item: EvidenceBundleItem, idx: number) => (
              <button
                key={idx}
                type="button"
                className="threaded-chat-bundle-item threaded-chat-bundle-item-clickable"
                onClick={() => drillIntoEvidence(msg.id, item)}
                title="Drill into this evidence in a sibling thread"
              >
                <span className="threaded-chat-bundle-source">[{item.source}]</span>{" "}
                <span className="threaded-chat-bundle-title">{item.title || item.source}</span>
                {item.refs && item.refs.length > 0 && (
                  <div className="threaded-chat-bundle-refs">{item.refs.join(", ")}</div>
                )}
              </button>
            ))}
          </div>
        );
      case "score_card":
        return (
          <div className="threaded-chat-score">
            {msg.payload?.metric}: {msg.payload?.value?.toFixed?.(2) ?? msg.payload?.value}
          </div>
        );
      case "action_chips":
        return (
          <div className="threaded-chat-chips">
            {(msg.payload?.chips || []).map(
              (chip: { label: string; command: string; disabled?: boolean }, idx: number) => (
                <button
                  key={idx}
                  type="button"
                  disabled={chip.disabled}
                  className="threaded-chat-chip"
                >
                  {chip.label}
                </button>
              ),
            )}
          </div>
        );
      default:
        return <div className="threaded-chat-fallback">[unsupported widget: {msg.type}]</div>;
    }
  };

  return (
    <aside className="threaded-chat-sidepane">
      <header className="threaded-chat-sidepane-header">
        <div className="threaded-chat-sidepane-eyebrow">thread</div>
        <button
          type="button"
          onClick={onClose}
          className="threaded-chat-sidepane-button"
          title="Hide pane"
        >
          ×
        </button>
      </header>
      <div className="threaded-chat-sidepane-messages">
        {/* Parent: render the chat message that anchors the thread,
            using the same BlockRenderer the main chat uses. This is
            the "head" of the thread — every follow-up below it is
            scoped to this context. */}
        {parentMessage && (
          <div className="threaded-chat-parent">
            <div className="threaded-chat-message-meta">{roleLabel(parentMessage.role as Role)} · parent</div>
            <div className="threaded-chat-parent-body">
              {parentMessage.blocks.map((block, i) => (
                <BlockRenderer
                  key={i}
                  block={block}
                  isLast={i === parentMessage.blocks.length - 1}
                  streaming={false}
                  onAction={onAction}
                />
              ))}
            </div>
            <div className="threaded-chat-parent-divider">replies</div>
          </div>
        )}
        {messages.length === 0 && !pending && !parentMessage && (
          <div className="threaded-chat-empty">ask a follow-up about this message.</div>
        )}
        {!threadID && (
          <div className="threaded-chat-thinking">
            <span className="threaded-chat-thinking-dot" />
            <span className="threaded-chat-thinking-dot" />
            <span className="threaded-chat-thinking-dot" />
            <span className="threaded-chat-thinking-label">preparing thread…</span>
          </div>
        )}
        {messages.map((msg) => (
          <div key={msg.id} className={`threaded-chat-message threaded-chat-${msg.role}`}>
            <div className="threaded-chat-message-meta">{roleLabel(msg.role)}</div>
            {renderPayload(msg)}
          </div>
        ))}
        {/* Optimistic pending row: user's typed line + thinking pill.
            Rendered immediately on submit so the pane never feels
            unresponsive while the loop is doing its plan→draft work. */}
        {pending && (
          <>
            <div className="threaded-chat-message threaded-chat-user">
              <div className="threaded-chat-message-meta">you</div>
              <div className="threaded-chat-text">{pending.body}</div>
            </div>
            <div className="threaded-chat-thinking">
              <span className="threaded-chat-thinking-dot" />
              <span className="threaded-chat-thinking-dot" />
              <span className="threaded-chat-thinking-dot" />
              <span className="threaded-chat-thinking-label">researching…</span>
            </div>
          </>
        )}
      </div>
      <div className="threaded-chat-sidepane-input-slot">
        {/* Disable input while the spawn round-trip is still in
            flight (threadID empty) so the user doesn't type into
            a void. The pane shows the parent message + a "preparing
            thread…" hint until the real id resolves. */}
        {renderInput(dispatchInThread, busy || !threadID)}
      </div>
    </aside>
  );
}

// clamp truncates s to n characters and appends an ellipsis when the
// original was longer. Whitespace is normalised to single spaces so a
// thread title rendered from a multi-line user message stays on one
// line in the side-pane header.
function clamp(s: string, n: number): string {
  const flat = s.replace(/\s+/g, " ").trim();
  if (flat.length <= n) return flat;
  return flat.slice(0, n - 1) + "…";
}

// roleLabel maps the wire role values onto the gl1tch voice. We never
// say "assistant" in user-facing copy — the assistant has a name and
// it is gl1tch. The label is also lowercase to match the rest of the
// chat-meta typography.
function roleLabel(role: Role): string {
  switch (role) {
    case "user":
      return "you";
    case "assistant":
      return "gl1tch";
    case "system":
      return "system";
    default:
      return role;
  }
}

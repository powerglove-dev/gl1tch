import { Fragment, useEffect, useRef, type CSSProperties } from "react";
import { Cpu, Terminal } from "lucide-react";
import type { Message, Block } from "@/lib/types";
import { formatTime12, dayLabel, isNewDay } from "@/lib/time";
import {
  TextBlock,
  CodeBlock,
  TableBlock,
  ActionBlock,
  StatusBlock,
  LinkCard,
  ErrorBlock,
  ChainBlock,
  ActivityBlock,
  BrainBlock,
} from "./blocks";

interface Props {
  messages: Message[];
  onAction: (method: string, args?: unknown[]) => Promise<void>;
  /** Transient status text rendered as a "gl1tch is thinking" pill while
   *  the latest assistant message is streaming. Empty string hides it. */
  thinking?: string;
  /** Click-to-thread handler. Every message row gets a hover-revealed
   *  thread button that calls this with the message ID. The host App
   *  spawns a thread under that ID and opens its side pane. */
  onOpenThread?: (messageID: string) => void;
  /** ID of the message whose thread is currently open in the side pane,
   *  if any. Used to highlight the row so the user can see which
   *  message anchors the visible thread. */
  activeThreadParentID?: string;
}

// BlockRenderer is exported so the thread side pane can render the
// parent chat message at the top of a thread using the exact same
// block components the main chat uses. Keeping one renderer means a
// drop-down code block, an activity row, or a brain note all look
// identical inside a thread and outside it.
export function BlockRenderer({
  block,
  isLast,
  streaming,
  onAction,
}: {
  block: Block;
  isLast: boolean;
  streaming?: boolean;
  onAction: (method: string, args?: unknown[]) => Promise<void>;
}) {
  switch (block.type) {
    case "text":
      return <TextBlock content={block.content} streaming={isLast && streaming} />;
    case "code":
      return <CodeBlock content={block.content} language={block.language} filename={block.filename} />;
    case "chain":
      return <ChainBlock steps={block.steps} text={block.text} />;
    case "activity":
      return <ActivityBlock summary={block.summary} tools={block.tools} />;
    case "brain":
      return <BrainBlock note={block.note} />;
    case "table":
      return <TableBlock headers={block.headers} rows={block.rows} />;
    case "action":
      return <ActionBlock id={block.id} label={block.label} method={block.method} args={block.args} onAction={onAction} />;
    case "status":
      return <StatusBlock text={block.text} />;
    case "link":
      return <LinkCard url={block.url} title={block.title} description={block.description} />;
    case "error":
      return <ErrorBlock message={block.message} />;
    case "done":
      return null;
    default:
      return null;
  }
}

function MessageRow({
  message,
  onAction,
  onOpenThread,
  isActiveThreadParent,
}: {
  message: Message;
  onAction: (method: string, args?: unknown[]) => Promise<void>;
  onOpenThread?: (messageID: string) => void;
  isActiveThreadParent?: boolean;
}) {
  const isUser = message.role === "user";
  const doneMeta = message.blocks.find((b) => b.type === "done");
  // Chain messages render their own chip layout — skip the user chat bubble
  // styling so the pills aren't wrapped in a bordered box.
  const isChainOnly =
    isUser && message.blocks.length === 1 && message.blocks[0].type === "chain";

  // Injected messages carry an event_key from the backend so the
  // activity sidebar's "↗ in chat" affordance can find this row
  // and scroll to it. We surface the key as a data attribute
  // because that's the cheapest lookup from a window-level scroll
  // event (no ref book-keeping, no store-level scroll target, just
  // querySelectorAll by attribute).
  const injectedKey = message.injected?.event_key;

  return (
    <div
      className={`fade-in glitch-message-row${isActiveThreadParent ? " glitch-message-row-active" : ""}`}
      style={{
        display: "flex",
        gap: 10,
        maxWidth: 760,
        marginLeft: isUser ? "auto" : 0,
        position: "relative",
      }}
      data-event-key={injectedKey || undefined}
    >
      {/* Avatar */}
      {!isUser && (
        <div
          style={{
            width: 28,
            height: 28,
            borderRadius: 8,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: "linear-gradient(135deg, var(--bg-surface) 0%, var(--bg-elevated) 100%)",
            border: "1px solid var(--border)",
            flexShrink: 0,
          }}
        >
          <Cpu size={13} style={{ color: "var(--cyan)" }} />
        </div>
      )}

      {/* Content */}
      <div style={{ minWidth: 0, flex: 1 }}>
        {/* Label */}
        <div
          style={{
            fontSize: 10,
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.08em",
            marginBottom: 4,
            color: isUser ? "var(--fg-dim)" : "var(--cyan)",
            textAlign: isUser ? "right" : "left",
          }}
        >
          <span style={{ display: "inline-flex", alignItems: "baseline", gap: 6 }}>
            <span>{isUser ? "you" : "gl1tch"}</span>
            {message.timestamp > 0 && (
              <span
                style={{
                  fontWeight: 400,
                  textTransform: "none",
                  letterSpacing: 0,
                  color: "var(--fg-dim)",
                  opacity: 0.7,
                  fontSize: 10,
                }}
                title={new Date(message.timestamp).toLocaleString()}
              >
                {formatTime12(message.timestamp)}
              </span>
            )}
          </span>
        </div>

        {/* Message body */}
        <div
          style={
            isUser && !isChainOnly
              ? {
                  display: "inline-block",
                  float: "right" as const,
                  background: "var(--bg-surface)",
                  border: "1px solid var(--border)",
                  borderRadius: "12px 12px 4px 12px",
                  padding: "8px 14px",
                  fontSize: 13,
                  lineHeight: 1.5,
                  maxWidth: "100%",
                }
              : { fontSize: 13 }
          }
        >
          {message.blocks.map((block, i) => (
            <BlockRenderer
              key={i}
              block={block}
              isLast={i === message.blocks.length - 1}
              streaming={message.streaming}
              onAction={onAction}
            />
          ))}
        </div>

        {/* Done metadata */}
        {doneMeta && doneMeta.type === "done" && doneMeta.model && (
          <div
            style={{
              marginTop: 6,
              fontSize: 10,
              color: "var(--fg-dim)",
              opacity: 0.6,
              clear: "both" as const,
            }}
          >
            {doneMeta.model}
            {doneMeta.tokens != null && ` \u00b7 ${doneMeta.tokens} tokens`}
            {doneMeta.latency_ms != null && ` \u00b7 ${(doneMeta.latency_ms / 1000).toFixed(1)}s`}
          </div>
        )}
      </div>

      {/* User avatar */}
      {isUser && (
        <div
          style={{
            width: 28,
            height: 28,
            borderRadius: 8,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: "var(--bg-surface)",
            border: "1px solid var(--border)",
            flexShrink: 0,
          }}
        >
          <Terminal size={13} style={{ color: "var(--green)" }} />
        </div>
      )}

      {/* Hover-revealed thread button. Every message in the chat is a
          potential thread anchor — clicking opens a side pane with its
          own input that runs research-grounded follow-ups against the
          gl1tch loop. The handler is wired by App so the side pane is
          a layout-level concern, not a per-row one. */}
      {onOpenThread && message.id && (
        <button
          type="button"
          className="glitch-thread-affordance"
          onClick={() => onOpenThread(message.id)}
          title="Open thread on this message"
        >
          💬
        </button>
      )}
    </div>
  );
}

export function MessageList({ messages, onAction, thinking, onOpenThread, activeThreadParentID }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, thinking]);

  // Listen for "jump to this injected message" requests from the
  // activity sidebar's "↗ in chat" affordance. The dispatcher emits
  // a window-level custom event with { event_key }; we look up the
  // matching MessageRow by its data-event-key attribute and scroll
  // it into view. Brief flash treatment so the user's eye lands on
  // the right row rather than hunting up the scroll column.
  useEffect(() => {
    const onJump = (ev: Event) => {
      const ce = ev as CustomEvent<{ event_key?: string }>;
      const key = ce?.detail?.event_key;
      if (!key || !scrollRef.current) return;
      const target = scrollRef.current.querySelector<HTMLDivElement>(
        `[data-event-key="${CSS.escape(key)}"]`,
      );
      if (!target) return;
      target.scrollIntoView({ behavior: "smooth", block: "center" });
      target.style.transition = "outline 0.3s";
      target.style.outline = "2px solid var(--purple, #bd93f9)";
      target.style.outlineOffset = "4px";
      setTimeout(() => {
        target.style.outline = "none";
      }, 1500);
    };
    window.addEventListener("glitch:scroll-to-chat", onJump as EventListener);
    return () => window.removeEventListener("glitch:scroll-to-chat", onJump as EventListener);
  }, []);

  if (messages.length === 0) {
    return (
      <div
        style={{
          flex: 1,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          gap: 8,
        }}
      >
        <div
          style={{
            width: 48,
            height: 48,
            borderRadius: 14,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: "linear-gradient(135deg, var(--bg-surface) 0%, var(--bg-elevated) 100%)",
            border: "1px solid var(--border)",
            marginBottom: 4,
          }}
        >
          <Cpu size={22} style={{ color: "var(--cyan)" }} />
        </div>
        <div style={{ fontSize: 18, fontWeight: 600, color: "var(--fg-bright)" }}>
          gl1tch
        </div>
        <div style={{ fontSize: 12, color: "var(--fg-dim)", maxWidth: 280, textAlign: "center", lineHeight: 1.5 }}>
          Ask about your repos, agents, and activity across monitored directories
        </div>
      </div>
    );
  }

  return (
    <div ref={scrollRef} style={{ flex: 1, overflowY: "auto", padding: "20px 24px" }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        {messages.map((msg, i) => {
          // Insert a day separator the first time we see a message
          // and every time the calendar day changes between rows.
          const prev = messages[i - 1];
          const showSeparator =
            i === 0 || isNewDay(prev?.timestamp ?? 0, msg.timestamp);
          return (
            <Fragment key={msg.id}>
              {showSeparator && msg.timestamp > 0 && (
                <DaySeparator label={dayLabel(msg.timestamp)} />
              )}
              <MessageRow
                message={msg}
                onAction={onAction}
                onOpenThread={onOpenThread}
                isActiveThreadParent={activeThreadParentID === msg.id}
              />
            </Fragment>
          );
        })}
        {thinking && <ThinkingPill text={thinking} />}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

/**
 * "── Today ──" style divider injected between messages on day
 * boundaries. Subtle so it never competes with chat content but
 * always visible enough to anchor the user in time.
 */
function DaySeparator({ label }: { label: string }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        margin: "4px 0 -4px",
      }}
      aria-label={`Day: ${label}`}
    >
      <span style={{ flex: 1, height: 1, background: "var(--border)", opacity: 0.6 }} />
      <span
        style={{
          fontSize: 10,
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.12em",
          color: "var(--fg-dim)",
          opacity: 0.8,
          padding: "2px 10px",
          borderRadius: 999,
          border: "1px solid var(--border)",
          background: "var(--bg-dark)",
        }}
      >
        {label}
      </span>
      <span style={{ flex: 1, height: 1, background: "var(--border)", opacity: 0.6 }} />
    </div>
  );
}

/**
 * Compact "gl1tch is thinking…" indicator shown while the active assistant
 * message is streaming. Text comes from <<GLITCH_STATUS>> events emitted by
 * the agent, falling back to a generic "thinking…" placeholder.
 */
function ThinkingPill({ text }: { text: string }) {
  return (
    <div className="fade-in" style={{ display: "flex", gap: 10, maxWidth: 760 }}>
      <div
        style={{
          width: 28,
          height: 28,
          borderRadius: 8,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "linear-gradient(135deg, var(--bg-surface) 0%, var(--bg-elevated) 100%)",
          border: "1px solid var(--border)",
          flexShrink: 0,
        }}
      >
        <Cpu size={13} style={{ color: "var(--cyan)" }} />
      </div>
      <div
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 8,
          padding: "6px 12px",
          borderRadius: 999,
          background: "var(--bg-surface)",
          border: "1px solid var(--border)",
          color: "var(--fg-dim)",
          fontSize: 12,
          alignSelf: "flex-start",
        }}
      >
        <ThinkingDots />
        <span>{text}</span>
      </div>
    </div>
  );
}

function ThinkingDots() {
  return (
    <span
      aria-hidden
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 3,
      }}
    >
      <span style={dotStyle(0)} />
      <span style={dotStyle(1)} />
      <span style={dotStyle(2)} />
    </span>
  );
}

function dotStyle(i: number): CSSProperties {
  return {
    width: 5,
    height: 5,
    borderRadius: "50%",
    background: "var(--cyan)",
    opacity: 0.4,
    animation: `glitch-dot 1.2s ${i * 0.15}s ease-in-out infinite`,
  };
}

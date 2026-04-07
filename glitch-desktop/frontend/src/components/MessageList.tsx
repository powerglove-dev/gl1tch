import { useEffect, useRef, type CSSProperties } from "react";
import { Cpu, Terminal } from "lucide-react";
import type { Message, Block } from "@/lib/types";
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
}

function BlockRenderer({
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
}: {
  message: Message;
  onAction: (method: string, args?: unknown[]) => Promise<void>;
}) {
  const isUser = message.role === "user";
  const doneMeta = message.blocks.find((b) => b.type === "done");
  // Chain messages render their own chip layout — skip the user chat bubble
  // styling so the pills aren't wrapped in a bordered box.
  const isChainOnly =
    isUser && message.blocks.length === 1 && message.blocks[0].type === "chain";

  return (
    <div className="fade-in" style={{ display: "flex", gap: 10, maxWidth: 760, marginLeft: isUser ? "auto" : 0 }}>
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
          {isUser ? "you" : "gl1tch"}
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
    </div>
  );
}

export function MessageList({ messages, onAction, thinking }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, thinking]);

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
    <div style={{ flex: 1, overflowY: "auto", padding: "20px 24px" }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        {messages.map((msg) => (
          <MessageRow key={msg.id} message={msg} onAction={onAction} />
        ))}
        {thinking && <ThinkingPill text={thinking} />}
        <div ref={bottomRef} />
      </div>
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

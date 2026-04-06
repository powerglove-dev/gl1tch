import { useEffect, useRef } from "react";
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
} from "./blocks";

interface Props {
  messages: Message[];
  onAction: (method: string, args?: unknown[]) => Promise<void>;
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
            isUser
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

export function MessageList({ messages, onAction }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

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
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

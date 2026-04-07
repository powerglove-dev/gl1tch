import { useState } from "react";
import { Check, X, ChevronDown, ChevronRight, Activity } from "lucide-react";
import type { ToolCall } from "@/lib/types";

interface Props {
  summary: string[];
  tools: ToolCall[];
}

/**
 * Renders the "run activity" frame for a CLI agent response: token/latency
 * summary pulled from the preamble, plus a collapsed list of tool calls the
 * agent attempted. Starts collapsed so the model's actual answer is what the
 * user sees first.
 */
export function ActivityBlock({ summary, tools }: Props) {
  const [open, setOpen] = useState(false);

  if (summary.length === 0 && tools.length === 0) return null;

  const okCount = tools.filter((t) => t.ok).length;
  const failCount = tools.length - okCount;

  return (
    <div
      style={{
        margin: "4px 0 10px",
        border: "1px solid var(--border)",
        borderRadius: 8,
        background: "var(--bg-surface)",
        overflow: "hidden",
      }}
    >
      <button
        onClick={() => setOpen((v) => !v)}
        style={{
          width: "100%",
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "8px 12px",
          background: "transparent",
          border: "none",
          color: "var(--fg-dim)",
          fontSize: 12,
          fontFamily: "inherit",
          cursor: "pointer",
          textAlign: "left",
        }}
        onMouseEnter={(e) => (e.currentTarget.style.color = "var(--fg)")}
        onMouseLeave={(e) => (e.currentTarget.style.color = "var(--fg-dim)")}
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        <Activity size={12} style={{ color: "var(--cyan)" }} />
        <span style={{ color: "var(--fg)" }}>Run activity</span>
        <span style={{ marginLeft: "auto", display: "flex", gap: 10, alignItems: "center" }}>
          {tools.length > 0 && (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
              {okCount > 0 && (
                <span style={{ color: "var(--green)" }}>
                  <Check size={11} style={{ verticalAlign: "-1px" }} /> {okCount}
                </span>
              )}
              {failCount > 0 && (
                <span style={{ color: "var(--red)" }}>
                  <X size={11} style={{ verticalAlign: "-1px" }} /> {failCount}
                </span>
              )}
            </span>
          )}
        </span>
      </button>

      {open && (
        <div style={{ borderTop: "1px solid var(--border)", padding: "10px 12px" }}>
          {summary.length > 0 && (
            <div style={{ marginBottom: tools.length > 0 ? 12 : 0 }}>
              <div
                style={{
                  fontSize: 10,
                  fontWeight: 600,
                  letterSpacing: "0.08em",
                  textTransform: "uppercase",
                  color: "var(--fg-dim)",
                  marginBottom: 4,
                }}
              >
                Summary
              </div>
              <ul style={{ listStyle: "none", margin: 0, padding: 0, fontSize: 12, lineHeight: 1.6 }}>
                {summary.map((line, i) => (
                  <li key={i} style={{ color: "var(--fg)", overflowWrap: "anywhere" }}>
                    {line}
                  </li>
                ))}
              </ul>
            </div>
          )}

          {tools.length > 0 && (
            <div>
              <div
                style={{
                  fontSize: 10,
                  fontWeight: 600,
                  letterSpacing: "0.08em",
                  textTransform: "uppercase",
                  color: "var(--fg-dim)",
                  marginBottom: 6,
                }}
              >
                Tools ({tools.length})
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {tools.map((t, i) => (
                  <ToolRow key={i} tool={t} />
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ToolRow({ tool }: { tool: ToolCall }) {
  return (
    <div
      style={{
        display: "flex",
        gap: 8,
        padding: "6px 8px",
        background: "var(--bg)",
        border: "1px solid var(--border)",
        borderRadius: 6,
        fontSize: 12,
      }}
    >
      <div style={{ flexShrink: 0, marginTop: 2 }}>
        {tool.ok ? (
          <Check size={12} style={{ color: "var(--green)" }} />
        ) : (
          <X size={12} style={{ color: "var(--red)" }} />
        )}
      </div>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ display: "flex", alignItems: "baseline", gap: 6, flexWrap: "wrap" }}>
          <span style={{ color: "var(--fg-bright)", fontWeight: 500 }}>{tool.label}</span>
          <span
            style={{
              fontSize: 10,
              padding: "1px 6px",
              borderRadius: 4,
              background: "var(--bg-surface)",
              color: "var(--fg-dim)",
              fontFamily: "monospace",
            }}
          >
            {tool.tool}
          </span>
        </div>
        {tool.command && (
          <div
            style={{
              marginTop: 4,
              fontFamily: "monospace",
              fontSize: 11,
              color: "var(--fg-dim)",
              overflowWrap: "anywhere",
              whiteSpace: "pre-wrap",
            }}
          >
            {tool.command}
          </div>
        )}
        {tool.result && (
          <div
            style={{
              marginTop: 4,
              fontSize: 11,
              color: tool.ok ? "var(--fg)" : "var(--red)",
              overflowWrap: "anywhere",
              whiteSpace: "pre-wrap",
            }}
          >
            {tool.result}
          </div>
        )}
      </div>
    </div>
  );
}

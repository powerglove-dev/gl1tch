import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import { Brain, AlertTriangle, Lightbulb, FileText, BookmarkCheck } from "lucide-react";
import type { BrainNote } from "@/lib/types";

interface Props {
  note: BrainNote;
}

const TYPE_META: Record<
  string,
  { color: string; bg: string; border: string; Icon: typeof Brain; label: string }
> = {
  finding: {
    color: "var(--orange)",
    bg: "rgba(255,158,100,0.06)",
    border: "rgba(255,158,100,0.35)",
    Icon: AlertTriangle,
    label: "Finding",
  },
  insight: {
    color: "var(--yellow)",
    bg: "rgba(224,175,104,0.06)",
    border: "rgba(224,175,104,0.35)",
    Icon: Lightbulb,
    label: "Insight",
  },
  note: {
    color: "var(--cyan)",
    bg: "rgba(125,207,255,0.06)",
    border: "rgba(125,207,255,0.35)",
    Icon: FileText,
    label: "Note",
  },
  decision: {
    color: "var(--green)",
    bg: "rgba(158,206,106,0.06)",
    border: "rgba(158,206,106,0.35)",
    Icon: BookmarkCheck,
    label: "Decision",
  },
};

const DEFAULT_META = {
  color: "var(--purple)",
  bg: "rgba(187,154,247,0.06)",
  border: "rgba(187,154,247,0.35)",
  Icon: Brain,
  label: "Brain",
};

export function BrainBlock({ note }: Props) {
  const meta = (note.brainType && TYPE_META[note.brainType]) || DEFAULT_META;
  const Icon = meta.Icon;

  return (
    <div
      style={{
        margin: "10px 0",
        border: `1px solid ${meta.border}`,
        borderLeft: `3px solid ${meta.color}`,
        borderRadius: 8,
        background: meta.bg,
        overflow: "hidden",
      }}
    >
      {/* Header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "8px 12px",
          borderBottom: `1px solid ${meta.border}`,
        }}
      >
        <Icon size={13} style={{ color: meta.color, flexShrink: 0 }} />
        <span
          style={{
            fontSize: 10,
            fontWeight: 600,
            letterSpacing: "0.08em",
            textTransform: "uppercase",
            color: meta.color,
          }}
        >
          {meta.label}
        </span>
        {note.title && (
          <span
            style={{
              fontSize: 12,
              fontWeight: 500,
              color: "var(--fg-bright)",
              minWidth: 0,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {note.title}
          </span>
        )}
        {note.tags.length > 0 && (
          <div style={{ marginLeft: "auto", display: "flex", gap: 4, flexWrap: "wrap", justifyContent: "flex-end" }}>
            {note.tags.map((tag) => (
              <span
                key={tag}
                style={{
                  fontSize: 10,
                  padding: "1px 6px",
                  borderRadius: 4,
                  background: "var(--bg)",
                  border: "1px solid var(--border)",
                  color: "var(--fg-dim)",
                  fontFamily: "monospace",
                }}
              >
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Body */}
      <div className="prose" style={{ padding: "10px 12px", fontSize: 13 }}>
        <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]}>{note.content}</ReactMarkdown>
      </div>
    </div>
  );
}

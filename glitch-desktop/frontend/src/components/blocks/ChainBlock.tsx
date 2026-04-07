import { ArrowRight, Bot, Workflow, MessageSquare } from "lucide-react";
import type { ChainStepChip } from "@/lib/types";

interface Props {
  steps: ChainStepChip[];
  text?: string;
}

const KIND_META: Record<
  ChainStepChip["kind"],
  { color: string; bg: string; border: string; Icon: typeof Bot }
> = {
  prompt: { color: "var(--cyan)", bg: "rgba(125,207,255,0.08)", border: "rgba(125,207,255,0.35)", Icon: MessageSquare },
  agent: { color: "var(--purple)", bg: "rgba(187,154,247,0.08)", border: "rgba(187,154,247,0.35)", Icon: Bot },
  pipeline: { color: "var(--green)", bg: "rgba(158,206,106,0.08)", border: "rgba(158,206,106,0.35)", Icon: Workflow },
};

export function ChainBlock({ steps, text }: Props) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 8,
        alignItems: "flex-end",
      }}
    >
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: 6,
          alignItems: "center",
          justifyContent: "flex-end",
        }}
      >
        {steps.map((step, i) => {
          const meta = KIND_META[step.kind] ?? KIND_META.prompt;
          const Icon = meta.Icon;
          // For prompt steps, surface the resolved provider (and model when
          // known) as a small badge appended to the chip. This makes the
          // actual executor visible before the run starts — no more silent
          // fallback to copilot. We render `provider/model` when both are
          // present so users can tell qwen3:8b from llama3.2 at a glance.
          const showProvider = step.kind === "prompt" && !!step.provider;
          const providerBadge =
            step.provider && step.model
              ? `${step.provider}/${step.model}`
              : step.provider || "";
          return (
            <div key={i} style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <div
                title={step.provider ? `${step.kind} · ${step.provider}` : step.kind}
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 6,
                  padding: "4px 10px",
                  borderRadius: 999,
                  background: meta.bg,
                  border: `1px solid ${meta.border}`,
                  color: meta.color,
                  fontSize: 12,
                  fontWeight: 500,
                  lineHeight: 1.2,
                  whiteSpace: "nowrap",
                }}
              >
                <Icon size={12} />
                {step.label}
                {showProvider && (
                  <span
                    style={{
                      marginLeft: 2,
                      padding: "0 6px",
                      borderRadius: 999,
                      background: "rgba(0,0,0,0.25)",
                      color: "var(--fg-dim)",
                      fontSize: 10,
                      fontWeight: 500,
                      fontFamily: "monospace",
                    }}
                  >
                    {providerBadge}
                  </span>
                )}
              </div>
              {i < steps.length - 1 && (
                <ArrowRight size={12} style={{ color: "var(--fg-dim)" }} />
              )}
            </div>
          );
        })}
      </div>
      {text && text.trim() && (
        <div
          style={{
            background: "var(--bg-surface)",
            border: "1px solid var(--border)",
            borderRadius: "12px 12px 4px 12px",
            padding: "8px 14px",
            fontSize: 13,
            lineHeight: 1.5,
            color: "var(--fg)",
            maxWidth: "100%",
            whiteSpace: "pre-wrap",
            overflowWrap: "anywhere",
          }}
        >
          {text}
        </div>
      )}
    </div>
  );
}

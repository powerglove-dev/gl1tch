import { Brain } from "lucide-react";
import type { SystemStatus } from "@/lib/types";

interface Props {
  status: SystemStatus;
  model?: string;
  tokens?: number;
}

function StatusDot({ on, label }: { on: boolean; label: string }) {
  return (
    <div className="flex items-center gap-1" style={{ fontSize: 11 }}>
      <div
        style={{
          width: 5,
          height: 5,
          borderRadius: "50%",
          background: on ? "var(--green)" : "var(--fg-dim)",
          opacity: on ? 1 : 0.4,
          boxShadow: on ? "0 0 4px var(--green)" : "none",
        }}
      />
      <span style={{ color: "var(--fg-dim)", opacity: on ? 0.8 : 0.4 }}>
        {label}
      </span>
    </div>
  );
}

export function Statusbar({ status, model, tokens }: Props) {
  const improving = status.brain === "improving";

  return (
    <div
      style={{
        height: 26,
        background: "var(--bg-dark)",
        borderTop: "1px solid var(--border)",
        display: "flex",
        alignItems: "center",
        padding: "0 10px",
        gap: 12,
        fontSize: 11,
        color: "var(--fg-dim)",
      }}
    >
      <StatusDot on={status.ollama} label="ollama" />
      <StatusDot on={status.elasticsearch} label="observer" />
      <StatusDot on={status.busd} label="busd" />

      {/* Brain indicator */}
      <div
        className="flex items-center gap-1"
        style={{ fontSize: 11 }}
        title={improving ? status.brainDetail : "Brain idle"}
      >
        <Brain
          size={11}
          style={{
            color: improving ? "var(--purple)" : "var(--fg-dim)",
            opacity: improving ? 1 : 0.4,
            animation: improving ? "pulse 2s ease-in-out infinite" : "none",
          }}
        />
        <span
          style={{
            color: improving ? "var(--purple)" : "var(--fg-dim)",
            opacity: improving ? 0.9 : 0.4,
          }}
        >
          {improving ? status.brainDetail || "improving" : "brain"}
        </span>
      </div>

      <div style={{ flex: 1 }} />
      {model && <span style={{ opacity: 0.6 }}>{model}</span>}
      {tokens != null && tokens > 0 && (
        <span style={{ opacity: 0.5 }}>{tokens} tokens</span>
      )}
    </div>
  );
}

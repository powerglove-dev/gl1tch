import { PanelLeft, Settings } from "lucide-react";
import type { SystemStatus } from "@/lib/types";

interface Props {
  status: SystemStatus;
  sidebarOpen: boolean;
  onToggleSidebar: () => void;
}

export function Titlebar({ status, sidebarOpen, onToggleSidebar }: Props) {
  const online = status.ollama || status.elasticsearch || status.busd;

  return (
    <div
      className="flex items-center justify-between px-3 select-none"
      style={{
        height: 44,
        background: "var(--bg-dark)",
        borderBottom: "1px solid var(--border)",
        WebkitAppRegion: "drag",
      } as React.CSSProperties}
    >
      <div className="flex items-center gap-2">
        <button
          onClick={onToggleSidebar}
          style={{
            WebkitAppRegion: "no-drag",
            background: "none",
            border: "none",
            color: "var(--fg-dim)",
            cursor: "pointer",
            padding: 4,
            borderRadius: 6,
            display: "flex",
            alignItems: "center",
          } as React.CSSProperties}
          title={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
        >
          <PanelLeft size={15} />
        </button>
        <span style={{ color: "var(--cyan)", fontWeight: 700, fontSize: 14, letterSpacing: "0.03em" }}>
          gl1tch
        </span>
      </div>

      <div
        className="flex items-center gap-2"
        style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
      >
        <div className="flex items-center gap-1.5" style={{ fontSize: 11, color: "var(--fg-dim)" }}>
          <div
            style={{
              width: 6,
              height: 6,
              borderRadius: "50%",
              background: online ? "var(--green)" : "var(--fg-dim)",
              boxShadow: online ? "0 0 6px var(--green)" : "none",
            }}
          />
          <span style={{ color: online ? "var(--green)" : "var(--fg-dim)" }}>
            {online ? "connected" : "offline"}
          </span>
        </div>
        <button
          style={{
            background: "none",
            border: "none",
            color: "var(--fg-dim)",
            cursor: "pointer",
            padding: 4,
            borderRadius: 6,
            display: "flex",
            alignItems: "center",
          }}
        >
          <Settings size={14} />
        </button>
      </div>
    </div>
  );
}

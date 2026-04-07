import { PanelLeft } from "lucide-react";
import { BrainIndicator } from "./BrainIndicator";
import type { BrainActivity, BrainState } from "@/lib/types";

interface Props {
  sidebarOpen: boolean;
  onToggleSidebar: () => void;
  brainState: BrainState;
  brainDetail: string;
  brainActivity: BrainActivity[];
  onMarkBrainRead: () => void;
  activeWorkspaceId: string | null;
  activeWorkspaceTitle: string;
}

const LOGO_LINES = [
  " ██████╗ ██╗     ██╗████████╗ ██████╗██╗  ██╗",
  "██╔════╝ ██║     ██║╚══██╔══╝██╔════╝██║  ██║",
  "██║  ███╗██║     ██║   ██║   ██║     ███████║",
  "██║   ██║██║     ██║   ██║   ██║     ██╔══██║",
  "╚██████╔╝███████╗██║   ██║   ╚██████╗██║  ██║",
  " ╚═════╝ ╚══════╝╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝",
];

export function Titlebar({
  sidebarOpen,
  onToggleSidebar,
  brainState,
  brainDetail,
  brainActivity,
  onMarkBrainRead,
  activeWorkspaceId,
  activeWorkspaceTitle,
}: Props) {
  return (
    <div
      className="flex items-center select-none"
      style={{
        height: 52,
        padding: "0 18px",
        background: "var(--bg-dark)",
        borderBottom: "1px solid var(--border)",
        WebkitAppRegion: "drag",
      } as React.CSSProperties}
    >
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
          marginRight: 8,
        } as React.CSSProperties}
        title={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
      >
        <PanelLeft size={15} />
      </button>
      <pre
        style={{
          fontFamily: "Berkeley Mono, JetBrains Mono, Fira Code, SF Mono, monospace",
          fontSize: 5.5,
          lineHeight: 1.0,
          color: "var(--cyan)",
          margin: 0,
          padding: 0,
          letterSpacing: "-0.02em",
          userSelect: "none",
        }}
        aria-label="gl1tch"
      >
        {LOGO_LINES.join("\n")}
      </pre>

      <div style={{ flex: 1 }} />

      {/* Persistent brain indicator — the only ambient status surface. */}
      <BrainIndicator
        state={brainState}
        detail={brainDetail}
        activity={brainActivity}
        onMarkRead={onMarkBrainRead}
        activeWorkspaceId={activeWorkspaceId}
        activeWorkspaceTitle={activeWorkspaceTitle}
      />
    </div>
  );
}

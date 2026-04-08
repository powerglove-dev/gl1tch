import { PanelLeft } from "lucide-react";
import { BrainIndicator } from "./BrainIndicator";
import type { BrainActivity, BrainState } from "@/lib/types";

interface Props {
  sidebarOpen: boolean;
  onToggleSidebar: () => void;
  /** Right-side activity sidebar visibility. Mirrors the left
   *  sidebar's toggle so the user can hide either side independently
   *  for a wider chat column. */
  activitySidebarOpen: boolean;
  onToggleActivitySidebar: () => void;
  brainState: BrainState;
  brainDetail: string;
  brainActivity: BrainActivity[];
  onMarkBrainRead: () => void;
  activeWorkspaceId: string | null;
  activeWorkspaceTitle: string;
  /** Open the EditorPopup on the active workspace's collectors.yaml.
   *  Plumbed through from App.tsx so the popup lives at the root and
   *  the brain indicator just emits the request. */
  onEditCollectors: () => void;
  /** Open the structured collector-config modal. Optional collector
   *  id pre-selects an entry; undefined lands on the first one. */
  onConfigureCollector: (collectorId?: string) => void;
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
  activitySidebarOpen,
  onToggleActivitySidebar,
  brainState,
  brainDetail,
  brainActivity,
  onMarkBrainRead,
  activeWorkspaceId,
  activeWorkspaceTitle,
  onEditCollectors,
  onConfigureCollector,
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
        onEditCollectors={onEditCollectors}
        onConfigureCollector={onConfigureCollector}
      />

      {/* Activity sidebar toggle retired 2026-04-08 — proactive
          signals now land as chat messages via emitChatInject,
          not in a separate panel. The props (activitySidebarOpen,
          onToggleActivitySidebar) are still accepted so App.tsx
          doesn't need to drop them in lockstep, but no button is
          rendered here. */}
    </div>
  );
}

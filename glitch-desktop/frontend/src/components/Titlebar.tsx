import { PanelLeft } from "lucide-react";

interface Props {
  sidebarOpen: boolean;
  onToggleSidebar: () => void;
}

const LOGO_LINES = [
  " ██████╗ ██╗     ██╗████████╗ ██████╗██╗  ██╗",
  "██╔════╝ ██║     ██║╚══██╔══╝██╔════╝██║  ██║",
  "██║  ███╗██║     ██║   ██║   ██║     ███████║",
  "██║   ██║██║     ██║   ██║   ██║     ██╔══██║",
  "╚██████╔╝███████╗██║   ██║   ╚██████╗██║  ██║",
  " ╚═════╝ ╚══════╝╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝",
];

export function Titlebar({ sidebarOpen, onToggleSidebar }: Props) {
  return (
    <div
      className="flex items-center px-4 select-none"
      style={{
        height: 52,
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
    </div>
  );
}

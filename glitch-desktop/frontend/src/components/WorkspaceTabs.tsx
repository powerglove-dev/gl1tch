import { Plus, Trash2 } from "lucide-react";
import { useState, useRef, useEffect } from "react";
import type { Workspace } from "@/lib/types";

interface Props {
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
  onSwitch: (id: string) => void;
  onDelete: (id: string) => void;
  onRename: (id: string, title: string) => void;
  onNew: () => void;
}

function WorkspaceTab({
  ws,
  isActive,
  onSwitch,
  onDelete,
  onRename,
}: {
  ws: Workspace;
  isActive: boolean;
  onSwitch: () => void;
  onDelete: () => void;
  onRename: (title: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [hovered, setHovered] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [editing]);

  function commit() {
    const val = inputRef.current?.value.trim();
    if (val && val !== ws.title) onRename(val);
    setEditing(false);
  }

  // Abbreviation: up to 2 chars from the title (uppercase)
  const abbr = ws.title
    .replace(/[^a-zA-Z0-9]/g, "")
    .slice(0, 2)
    .toUpperCase() || "W";

  return (
    <div
      onClick={() => { if (!editing) onSwitch(); }}
      onDoubleClick={(e) => { e.stopPropagation(); setEditing(true); }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      title={editing ? undefined : ws.primary_directory ? `${ws.title}\n${ws.primary_directory}` : ws.title}
      style={{
        position: "relative",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        width: "100%",
        padding: "10px 4px",
        cursor: "pointer",
        background: isActive ? "var(--bg-surface)" : hovered ? "rgba(125,207,255,0.04)" : "transparent",
        borderLeft: `2px solid ${isActive ? "var(--cyan)" : "transparent"}`,
        transition: "background 0.12s, border-color 0.12s",
        gap: 4,
      }}
    >
      {/* Abbr badge */}
      {!editing && (
        <div
          style={{
            width: 28,
            height: 28,
            borderRadius: 7,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontSize: 11,
            fontWeight: 700,
            letterSpacing: "0.04em",
            background: isActive ? "rgba(125,207,255,0.15)" : "var(--bg)",
            border: `1px solid ${isActive ? "var(--cyan)" : "var(--border)"}`,
            color: isActive ? "var(--cyan)" : "var(--fg-dim)",
            boxShadow: isActive ? "0 0 8px rgba(125,207,255,0.2)" : "none",
            transition: "all 0.12s",
            flexShrink: 0,
          }}
        >
          {abbr}
        </div>
      )}

      {/* Inline rename input */}
      {editing && (
        <input
          ref={inputRef}
          defaultValue={ws.title}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit();
            if (e.key === "Escape") setEditing(false);
          }}
          onClick={(e) => e.stopPropagation()}
          style={{
            width: 36,
            background: "var(--bg)",
            border: "1px solid var(--border-bright)",
            borderRadius: 4,
            padding: "2px 4px",
            color: "var(--fg)",
            fontSize: 10,
            fontFamily: "inherit",
            outline: "none",
            textAlign: "center",
          }}
        />
      )}

      {/* Delete button on hover */}
      {hovered && !editing && (
        <button
          onClick={(e) => { e.stopPropagation(); onDelete(); }}
          title={`Delete ${ws.title}`}
          style={{
            position: "absolute",
            top: 2,
            right: 2,
            width: 16,
            height: 16,
            background: "var(--bg-dark)",
            border: "1px solid var(--border)",
            color: "var(--red, #ff5555)",
            cursor: "pointer",
            padding: 0,
            borderRadius: 4,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            opacity: 1,
            lineHeight: 1,
            boxShadow: "0 1px 3px rgba(0,0,0,0.4)",
          }}
        >
          <Trash2 size={11} />
        </button>
      )}
    </div>
  );
}

export function WorkspaceTabs({
  workspaces,
  activeWorkspaceId,
  onSwitch,
  onDelete,
  onRename,
  onNew,
}: Props) {
  return (
    <div
      style={{
        width: 44,
        height: "100%",
        background: "var(--bg-dark)",
        borderRight: "1px solid var(--border)",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        paddingTop: 8,
        paddingBottom: 8,
        gap: 2,
        overflowY: "auto",
        overflowX: "hidden",
        flexShrink: 0,
      }}
    >
      {workspaces.map((ws) => (
        <WorkspaceTab
          key={ws.id}
          ws={ws}
          isActive={ws.id === activeWorkspaceId}
          onSwitch={() => onSwitch(ws.id)}
          onDelete={() => onDelete(ws.id)}
          onRename={(title) => onRename(ws.id, title)}
        />
      ))}

      {/* New workspace button */}
      <div style={{ flex: 1 }} />
      <button
        onClick={onNew}
        title="New workspace"
        style={{
          width: 28,
          height: 28,
          borderRadius: 7,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "transparent",
          border: "1px dashed var(--border)",
          color: "var(--fg-dim)",
          cursor: "pointer",
          flexShrink: 0,
        }}
      >
        <Plus size={12} />
      </button>
    </div>
  );
}

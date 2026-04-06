import {
  MessageSquare,
  Play,
  Bot,
  Activity,
  FolderOpen,
  Plus,
  ChevronRight,
  X,
  Folder,
  Trash2,
  Pencil,
} from "lucide-react";
import { useState, useRef, useEffect } from "react";
import type { Workspace } from "@/lib/types";

interface SidebarSectionProps {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode | ((filter: string) => React.ReactNode);
  defaultOpen?: boolean;
  count?: number;
}

function SidebarSection({ title, icon, children, defaultOpen = true, count, searchable }: SidebarSectionProps & { searchable?: boolean }) {
  const [open, setOpen] = useState(defaultOpen);
  const [search, setSearch] = useState("");

  return (
    <div style={{ marginBottom: 2 }}>
      <button
        onClick={() => setOpen(!open)}
        style={{
          width: "100%", display: "flex", alignItems: "center", gap: 6,
          padding: "6px 12px", fontSize: 11, fontWeight: 600,
          textTransform: "uppercase", letterSpacing: "0.06em",
          color: "var(--fg-dim)", background: "none", border: "none", cursor: "pointer",
        }}
      >
        <ChevronRight size={10} style={{ transform: open ? "rotate(90deg)" : "rotate(0)", transition: "transform 0.15s" }} />
        {icon}
        <span style={{ flex: 1, textAlign: "left" }}>{title}</span>
        {count != null && count > 0 && (
          <span style={{ fontSize: 10, background: "var(--bg-surface)", padding: "1px 6px", borderRadius: 8, color: "var(--fg-dim)" }}>{count}</span>
        )}
      </button>
      {open && (
        <div style={{ padding: "0 8px" }}>
          {searchable && count != null && count > 5 && (
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Filter..."
              style={{
                width: "100%", padding: "3px 8px", marginBottom: 4,
                background: "var(--bg)", border: "1px solid var(--border)",
                borderRadius: 4, color: "var(--fg)", fontSize: 11,
                outline: "none", fontFamily: "inherit",
              }}
            />
          )}
          {typeof children === "function" ? (children as (filter: string) => React.ReactNode)(search) : children}
        </div>
      )}
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return <div style={{ padding: "10px 12px", fontSize: 11, color: "var(--fg-dim)", fontStyle: "italic" }}>{text}</div>;
}

export interface PipelineEntry {
  name: string;
  description: string;
  path: string;
  project: string;
}

export interface AgentEntry {
  name: string;
  kind: string;
  source: string;
  description: string;
  invoke: string;
}

interface Props {
  onNewWorkspace: () => void;
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
  onSwitchWorkspace: (id: string) => void;
  onDeleteWorkspace: (id: string) => void;
  onRenameWorkspace: (id: string, title: string) => void;
  directories: string[];
  agents: AgentEntry[];
  pipelines: PipelineEntry[];
  selectedAgent: string | null;
  onAddDirectory: () => void;
  onRemoveDirectory: (dir: string) => void;
  onRunPipeline: (path: string) => void;
  onSelectAgent: (name: string) => void;
}

function WorkspaceItem({
  ws, isActive, onSwitch, onDelete, onRename,
}: {
  ws: Workspace; isActive: boolean;
  onSwitch: () => void; onDelete: () => void;
  onRename: (title: string) => void;
}) {
  const [editing, setEditing] = useState(false);
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

  return (
    <div
      onClick={() => { if (!editing) onSwitch(); }}
      onDoubleClick={(e) => { e.stopPropagation(); setEditing(true); }}
      style={{
        display: "flex", alignItems: "center", gap: 6,
        padding: "5px 8px", borderRadius: 6, fontSize: 12,
        color: isActive ? "var(--fg-bright)" : "var(--fg)",
        background: isActive ? "var(--bg-surface)" : "transparent",
        cursor: "pointer", transition: "background 0.1s",
      }}
    >
      <MessageSquare size={11} style={{ flexShrink: 0, opacity: 0.5 }} />
      {editing ? (
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
            flex: 1, background: "var(--bg)", border: "1px solid var(--border-bright)",
            borderRadius: 4, padding: "1px 4px", color: "var(--fg)", fontSize: 12,
            fontFamily: "inherit", outline: "none",
          }}
        />
      ) : (
        <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {ws.title}
        </span>
      )}
      {!editing && (
        <button
          onClick={(e) => { e.stopPropagation(); onDelete(); }}
          style={{ background: "none", border: "none", color: "var(--fg-dim)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.2 }}
        >
          <Trash2 size={10} />
        </button>
      )}
    </div>
  );
}

export function Sidebar({
  onNewWorkspace, workspaces, activeWorkspaceId,
  onSwitchWorkspace, onDeleteWorkspace, onRenameWorkspace,
  directories, agents, pipelines, selectedAgent,
  onAddDirectory, onRemoveDirectory, onRunPipeline, onSelectAgent,
}: Props) {
  return (
    <div style={{
      height: "100%", background: "var(--bg-dark)", borderRight: "1px solid var(--border)",
      display: "flex", flexDirection: "column", width: 220,
    }}>
      {/* New workspace button */}
      <div style={{ padding: "10px 10px 8px" }}>
        <button
          onClick={onNewWorkspace}
          style={{
            width: "100%", display: "flex", alignItems: "center", justifyContent: "center", gap: 6,
            padding: "7px 10px", borderRadius: 8, background: "var(--bg-surface)",
            border: "1px solid var(--border)", color: "var(--fg)", fontSize: 12, fontWeight: 500, cursor: "pointer",
          }}
        >
          <Plus size={13} />
          New Chat
        </button>
      </div>

      <div style={{ flex: 1, overflowY: "auto", paddingTop: 4 }}>
        {/* Per-workspace directories */}
        <SidebarSection title="Directories" icon={<FolderOpen size={11} />} count={directories.length}>
          {directories.length === 0 ? (
            <EmptyState text={activeWorkspaceId ? "No directories in this workspace" : "Create a chat first"} />
          ) : (
            directories.map((dir) => (
              <div
                key={dir}
                style={{
                  display: "flex", alignItems: "center", gap: 6,
                  padding: "4px 8px", borderRadius: 6, fontSize: 11, color: "var(--fg)",
                }}
              >
                <Folder size={11} style={{ color: "var(--yellow)", flexShrink: 0 }} />
                <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={dir}>
                  {dir.split("/").pop()}
                </span>
                <button
                  onClick={() => onRemoveDirectory(dir)}
                  style={{ background: "none", border: "none", color: "var(--fg-dim)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.3 }}
                  title={`Remove ${dir}`}
                >
                  <X size={10} />
                </button>
              </div>
            ))
          )}
          {activeWorkspaceId && (
            <button
              onClick={onAddDirectory}
              style={{
                display: "flex", alignItems: "center", gap: 5, padding: "5px 8px", marginTop: 2,
                borderRadius: 6, fontSize: 11, color: "var(--fg-dim)", background: "none", border: "none",
                cursor: "pointer", width: "100%",
              }}
            >
              <Plus size={10} />
              Add directory
            </button>
          )}
        </SidebarSection>

        {/* Workspaces / conversations */}
        <SidebarSection title="Workspaces" icon={<MessageSquare size={11} />} count={workspaces.length}>
          {workspaces.length === 0 ? (
            <EmptyState text="No workspaces yet" />
          ) : (
            workspaces.map((ws) => (
              <WorkspaceItem
                key={ws.id}
                ws={ws}
                isActive={ws.id === activeWorkspaceId}
                onSwitch={() => onSwitchWorkspace(ws.id)}
                onDelete={() => onDeleteWorkspace(ws.id)}
                onRename={(title) => onRenameWorkspace(ws.id, title)}
              />
            ))
          )}
        </SidebarSection>

        <SidebarSection title="Pipelines" icon={<Play size={11} />} count={pipelines.length} defaultOpen={pipelines.length > 0}>
          {pipelines.length === 0 ? (
            <EmptyState text="No pipelines in workspace dirs" />
          ) : (
            pipelines.map((p) => (
              <div
                key={p.path}
                onClick={() => onRunPipeline(p.path)}
                style={{
                  display: "flex", alignItems: "center", gap: 6,
                  padding: "4px 8px", borderRadius: 6, fontSize: 11,
                  color: "var(--fg)", cursor: "pointer",
                }}
                title={p.description + " (" + p.project + ")"}
              >
                <Play size={9} style={{ color: "var(--green)", flexShrink: 0 }} />
                <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {p.name}
                </span>
                <span style={{ fontSize: 9, color: "var(--fg-dim)" }}>{p.project}</span>
              </div>
            ))
          )}
        </SidebarSection>

        <SidebarSection title="Skills & Agents" icon={<Bot size={11} />} count={agents.length} defaultOpen={agents.length > 0} searchable>
          {(filter: string) => {
            const filtered = agents.filter((a) =>
              !filter || a.name.toLowerCase().includes(filter.toLowerCase())
            );
            if (filtered.length === 0) return <EmptyState text={agents.length ? "No matches" : "Add directories to discover"} />;
            return filtered.map((a) => (
              <div
                key={a.kind + ":" + a.name}
                onClick={() => onSelectAgent(a.name)}
                style={{
                  display: "flex", alignItems: "center", gap: 6,
                  padding: "4px 8px", borderRadius: 6, fontSize: 11,
                  color: selectedAgent === a.name ? "var(--fg-bright)" : "var(--fg)",
                  background: selectedAgent === a.name ? "var(--purple)" + "22" : "transparent",
                  cursor: "pointer",
                }}
                title={a.description}
              >
                <span style={{ color: a.kind === "skill" ? "var(--green)" : "var(--purple)", fontSize: 10, fontWeight: 600, width: 12 }}>
                  {a.kind === "skill" ? "/" : "@"}
                </span>
                <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {a.name}
                </span>
                <span style={{ fontSize: 9, color: "var(--fg-dim)" }}>{a.source}</span>
              </div>
            ));
          }}
        </SidebarSection>

        <SidebarSection title="Activity" icon={<Activity size={11} />} defaultOpen={false}>
          <EmptyState text="No recent activity" />
        </SidebarSection>
      </div>

      <div style={{ padding: "8px 12px", borderTop: "1px solid var(--border)", fontSize: 10, color: "var(--fg-dim)" }}>
        <kbd style={{ padding: "1px 5px", borderRadius: 4, background: "var(--bg-surface)", fontSize: 10, border: "1px solid var(--border)" }}>{"\u2318"}K</kbd> Commands
      </div>
    </div>
  );
}

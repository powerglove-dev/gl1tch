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
  FileText,
  Workflow,
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
  const [userToggled, setUserToggled] = useState(false);
  const [search, setSearch] = useState("");

  // Auto-expand once data arrives, unless the user has manually toggled.
  // This handles the common case where data loads after the section mounts.
  useEffect(() => {
    if (!userToggled && count != null && count > 0 && !open) {
      setOpen(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [count]);

  return (
    <div style={{ marginBottom: 6 }}>
      <button
        onClick={() => { setUserToggled(true); setOpen(!open); }}
        style={{
          width: "100%", display: "flex", alignItems: "center", gap: 8,
          padding: "8px 14px", fontSize: 11, fontWeight: 600,
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
        <div style={{ padding: "2px 10px 4px" }}>
          {searchable && count != null && count > 5 && (
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Filter..."
              style={{
                width: "100%", padding: "4px 10px", marginBottom: 6,
                background: "var(--bg)", border: "1px solid var(--border)",
                borderRadius: 6, color: "var(--fg)", fontSize: 11,
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
  return <div style={{ padding: "12px 14px", fontSize: 11, color: "var(--fg-dim)", fontStyle: "italic" }}>{text}</div>;
}

/**
 * A workflow file discovered on disk (.workflow.yaml or legacy .pipeline.yaml).
 * These are the "imported" half of the unified Workflows section, alongside
 * saved chain workflows from the DB (WorkflowEntry).
 */
export interface WorkflowFileEntry {
  name: string;
  description: string;
  path: string;
  workspace: string;
}

export interface AgentEntry {
  name: string;
  kind: string;
  source: string;
  description: string;
  invoke: string;
}

export interface PromptEntry {
  ID: number;
  Title: string;
  Body: string;
  ModelSlug: string;
  UpdatedAt: number;
}

export interface WorkflowEntry {
  ID: number;
  Name: string;
  StepsJSON: string;
  UpdatedAt: number;
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
  workflowFiles: WorkflowFileEntry[];
  prompts: PromptEntry[];
  workflows: WorkflowEntry[];
  selectedAgent: string | null;
  onAddDirectory: () => void;
  onRemoveDirectory: (dir: string) => void;
  onAddWorkflowFile: (p: WorkflowFileEntry) => void;
  onAddAgent: (name: string) => void;
  onAddPrompt: (p: PromptEntry) => void;
  onLoadWorkflow: (w: WorkflowEntry) => void;
  onRunWorkflow: (w: WorkflowEntry) => void;
  onDeleteWorkflow: (id: number) => void;
  onDeletePrompt: (id: number) => void;
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
        display: "flex", alignItems: "center", gap: 8,
        padding: "6px 10px", borderRadius: 6, fontSize: 12,
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
            borderRadius: 4, padding: "2px 6px", color: "var(--fg)", fontSize: 12,
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
  directories, agents, workflowFiles, prompts, workflows, selectedAgent,
  onAddDirectory, onRemoveDirectory, onAddWorkflowFile, onAddAgent,
  onAddPrompt, onLoadWorkflow, onRunWorkflow, onDeleteWorkflow, onDeletePrompt,
}: Props) {
  const totalWorkflows = workflows.length + workflowFiles.length;
  return (
    <div style={{
      height: "100%", background: "var(--bg-dark)", borderRight: "1px solid var(--border)",
      display: "flex", flexDirection: "column", width: 240,
    }}>
      {/* New workspace button */}
      <div style={{ padding: "12px 12px 10px" }}>
        <button
          onClick={onNewWorkspace}
          style={{
            width: "100%", display: "flex", alignItems: "center", justifyContent: "center", gap: 6,
            padding: "8px 12px", borderRadius: 8, background: "var(--bg-surface)",
            border: "1px solid var(--border)", color: "var(--fg)", fontSize: 12, fontWeight: 500, cursor: "pointer",
          }}
        >
          <Plus size={13} />
          New Chat
        </button>
      </div>

      <div style={{ flex: 1, overflowY: "auto", paddingTop: 4 }}>
        {/* Per-workspace directories */}
        <SidebarSection title="Directories" icon={<FolderOpen size={12} />} count={directories.length}>
          {directories.length === 0 ? (
            <EmptyState text={activeWorkspaceId ? "No directories in this workspace" : "Create a chat first"} />
          ) : (
            directories.map((dir) => (
              <div
                key={dir}
                style={{
                  display: "flex", alignItems: "center", gap: 8,
                  padding: "5px 10px", borderRadius: 6, fontSize: 11, color: "var(--fg)",
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
                display: "flex", alignItems: "center", gap: 6, padding: "6px 10px", marginTop: 4,
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
        <SidebarSection title="Workspaces" icon={<MessageSquare size={12} />} count={workspaces.length}>
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

        <SidebarSection title="Workflows" icon={<Workflow size={12} />} count={totalWorkflows} defaultOpen={totalWorkflows > 0}>
          {totalWorkflows === 0 ? (
            <EmptyState text="No workflows yet — build one in the chain bar" />
          ) : (
            <>
              {/* Saved chat workflows (user-built) */}
              {workflows.map((w) => (
                <div
                  key={"wf-" + w.ID}
                  onClick={() => onLoadWorkflow(w)}
                  style={{
                    display: "flex", alignItems: "center", gap: 8,
                    padding: "5px 10px", borderRadius: 6, fontSize: 11,
                    color: "var(--fg)", cursor: "pointer",
                  }}
                  title="Click to load into builder"
                >
                  <Workflow size={10} style={{ color: "var(--cyan)", flexShrink: 0 }} />
                  <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {w.Name}
                  </span>
                  <button
                    onClick={(e) => { e.stopPropagation(); onRunWorkflow(w); }}
                    style={{ background: "none", border: "none", color: "var(--green)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.7 }}
                    title="Run immediately"
                  >
                    <Play size={9} />
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); onDeleteWorkflow(w.ID); }}
                    style={{ background: "none", border: "none", color: "var(--fg-dim)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.2 }}
                  >
                    <Trash2 size={9} />
                  </button>
                </div>
              ))}
              {/* Workflow files discovered on disk (.workflow.yaml) */}
              {workflowFiles.map((p) => (
                <div
                  key={"wf-file-" + p.path}
                  onClick={() => onAddWorkflowFile(p)}
                  style={{
                    display: "flex", alignItems: "center", gap: 8,
                    padding: "5px 10px", borderRadius: 6, fontSize: 11,
                    color: "var(--fg)", cursor: "pointer",
                  }}
                  title={p.description + " · from " + p.workspace + " (click to add to builder)"}
                >
                  <Workflow size={10} style={{ color: "var(--cyan)", flexShrink: 0 }} />
                  <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {p.name}
                  </span>
                  <span style={{ fontSize: 9, color: "var(--fg-dim)" }}>{p.workspace}</span>
                </div>
              ))}
            </>
          )}
        </SidebarSection>

        <SidebarSection title="Prompts" icon={<FileText size={12} />} count={prompts.length} defaultOpen={prompts.length > 0} searchable>
          {(filter: string) => {
            const filtered = prompts.filter((p) =>
              !filter || p.Title.toLowerCase().includes(filter.toLowerCase())
            );
            if (filtered.length === 0) return <EmptyState text={prompts.length ? "No matches" : "No saved prompts"} />;
            return filtered.map((p) => (
              <div
                key={p.ID}
                onClick={() => onAddPrompt(p)}
                style={{
                  display: "flex", alignItems: "center", gap: 8,
                  padding: "5px 10px", borderRadius: 6, fontSize: 11,
                  color: "var(--fg)", cursor: "pointer",
                }}
                title={p.Body.length > 120 ? p.Body.slice(0, 120) + "..." : p.Body}
              >
                <FileText size={9} style={{ color: "var(--orange)", flexShrink: 0 }} />
                <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {p.Title}
                </span>
                {p.ModelSlug && <span style={{ fontSize: 9, color: "var(--fg-dim)" }}>{p.ModelSlug}</span>}
                <button
                  onClick={(e) => { e.stopPropagation(); onDeletePrompt(p.ID); }}
                  style={{ background: "none", border: "none", color: "var(--fg-dim)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.2 }}
                >
                  <Trash2 size={9} />
                </button>
              </div>
            ));
          }}
        </SidebarSection>

        <SidebarSection title="Skills & Agents" icon={<Bot size={12} />} count={agents.length} defaultOpen={agents.length > 0} searchable>
          {(filter: string) => {
            const filtered = agents.filter((a) =>
              !filter || a.name.toLowerCase().includes(filter.toLowerCase())
            );
            if (filtered.length === 0) return <EmptyState text={agents.length ? "No matches" : "Add directories to discover"} />;
            return filtered.map((a) => (
              <div
                key={a.kind + ":" + a.name}
                onClick={() => onAddAgent(a.name)}
                style={{
                  display: "flex", alignItems: "center", gap: 8,
                  padding: "5px 10px", borderRadius: 6, fontSize: 11,
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

        <SidebarSection title="Activity" icon={<Activity size={12} />} defaultOpen={false}>
          <EmptyState text="No recent activity" />
        </SidebarSection>
      </div>

      <div style={{ padding: "10px 14px", borderTop: "1px solid var(--border)", fontSize: 10, color: "var(--fg-dim)" }}>
        <kbd style={{ padding: "2px 6px", borderRadius: 4, background: "var(--bg-surface)", fontSize: 10, border: "1px solid var(--border)" }}>{"\u2318"}K</kbd> Commands
      </div>
    </div>
  );
}

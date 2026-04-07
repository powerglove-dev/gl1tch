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
  Pencil,
} from "lucide-react";
import { useState, useRef, useEffect } from "react";
import type { Workspace, BrainActivity } from "@/lib/types";
import { formatTime12 } from "@/lib/time";

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
  selectedAgent: string | null;
  brainActivity: BrainActivity[];
  onAddDirectory: () => void;
  onRemoveDirectory: (dir: string) => void;
  onAddWorkflowFile: (p: WorkflowFileEntry) => void;
  onRunWorkflowFile: (p: WorkflowFileEntry) => void;
  onDeleteWorkflowFile: (p: WorkflowFileEntry) => void;
  /** Open the EditorPopup with this workflow file loaded as a draft. */
  onEditWorkflowFile: (p: WorkflowFileEntry) => void;
  /** Open the EditorPopup on a brand-new workflow draft. */
  onNewWorkflow: () => void;
  onAddAgent: (name: string) => void;
  onAddPrompt: (p: PromptEntry) => void;
  onDeletePrompt: (id: number) => void;
  /** Open the EditorPopup with this prompt loaded as a draft. */
  onEditPrompt: (p: PromptEntry) => void;
  /** Open the EditorPopup on a brand-new prompt draft. */
  onNewPrompt: () => void;
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
  directories, agents, workflowFiles, prompts, selectedAgent,
  brainActivity,
  onAddDirectory, onRemoveDirectory, onAddWorkflowFile,
  onRunWorkflowFile, onDeleteWorkflowFile, onEditWorkflowFile, onNewWorkflow,
  onAddAgent, onAddPrompt, onDeletePrompt, onEditPrompt, onNewPrompt,
}: Props) {
  const totalWorkflows = workflowFiles.length;
  const activeWs = workspaces.find((w) => w.id === activeWorkspaceId);
  const contextLabel = activeWs?.title ?? "no workspace";
  const noWorkspace = !activeWorkspaceId;

  return (
    <div style={{
      height: "100%", background: "var(--bg-dark)", borderRight: "1px solid var(--border)",
      display: "flex", flexDirection: "column", width: 240,
    }}>
      <div style={{ flex: 1, overflowY: "auto" }}>
        {/* ── WORKSPACES (primary, top) ──────────────────────────────────
           Workspaces are the anchor of everything below. Visually heavier
           than the contextual sections so the user reads top-down:
           "I'm in workspace X → here's what X has." */}
        <div
          style={{
            padding: "20px 18px 14px",
            borderBottom: "1px solid var(--border)",
            background: "linear-gradient(180deg, rgba(125,207,255,0.04), transparent)",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              padding: "0 2px 10px",
              fontSize: 11,
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.08em",
              color: "var(--cyan)",
            }}
          >
            <MessageSquare size={12} />
            <span style={{ flex: 1 }}>workspaces</span>
            <span
              style={{
                fontSize: 10,
                background: "var(--bg-surface)",
                padding: "1px 6px",
                borderRadius: 8,
                color: "var(--fg-dim)",
              }}
            >
              {workspaces.length}
            </span>
          </div>

          {workspaces.length === 0 ? (
            <EmptyState text="No workspaces yet" />
          ) : (
            <div style={{ padding: "0 2px" }}>
              {workspaces.map((ws) => (
                <WorkspaceItem
                  key={ws.id}
                  ws={ws}
                  isActive={ws.id === activeWorkspaceId}
                  onSwitch={() => onSwitchWorkspace(ws.id)}
                  onDelete={() => onDeleteWorkspace(ws.id)}
                  onRename={(title) => onRenameWorkspace(ws.id, title)}
                />
              ))}
            </div>
          )}

          <button
            onClick={onNewWorkspace}
            style={{
              width: "100%", display: "flex", alignItems: "center", justifyContent: "center", gap: 6,
              padding: "8px 12px", marginTop: 12, borderRadius: 8,
              background: "transparent",
              border: "1px dashed var(--border)",
              color: "var(--fg-dim)",
              fontSize: 11, fontWeight: 500, cursor: "pointer",
            }}
          >
            <Plus size={11} />
            New workspace
          </button>
        </div>

        {/* ── CONTEXT FOR ACTIVE WORKSPACE ────────────────────────────────
           Everything below scopes to the active workspace. The contextual
           header makes that dependency obvious so users don't wonder why
           Directories/Workflows/etc. changed when they switched. */}
        <div
          style={{
            padding: "14px 18px 6px",
            display: "flex",
            alignItems: "center",
            gap: 6,
            fontSize: 10,
            textTransform: "uppercase",
            letterSpacing: "0.08em",
            color: "var(--fg-dim)",
            opacity: noWorkspace ? 0.5 : 0.85,
          }}
        >
          <span>context ·</span>
          <span
            style={{
              color: noWorkspace ? "var(--fg-dim)" : "var(--fg)",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              maxWidth: 160,
            }}
            title={contextLabel}
          >
            {contextLabel}
          </span>
        </div>

        <div
          style={{
            paddingTop: 2,
            // Subtle dim when no workspace selected — visually telegraphs
            // that these sections require a workspace.
            opacity: noWorkspace ? 0.5 : 1,
            transition: "opacity 0.15s",
          }}
        >
        {/* Per-workspace directories */}
        <SidebarSection title="Directories" icon={<FolderOpen size={12} />} count={directories.length}>
          {directories.length === 0 ? (
            <EmptyState text={activeWorkspaceId ? `No directories in ${contextLabel}` : "Create a workspace first"} />
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

        <SidebarSection title="Workflows" icon={<Workflow size={12} />} count={totalWorkflows} defaultOpen={totalWorkflows > 0}>
          {activeWorkspaceId && (
            <button
              onClick={onNewWorkflow}
              style={{
                display: "flex", alignItems: "center", gap: 6,
                padding: "5px 10px", marginBottom: 2,
                fontSize: 11, color: "var(--fg-dim)",
                background: "none", border: "none", borderRadius: 6,
                cursor: "pointer", width: "100%",
              }}
              title="Draft a new workflow"
            >
              <Plus size={10} />
              New workflow
            </button>
          )}
          {totalWorkflows === 0 ? (
            <EmptyState text="No workflows yet — build one in the chain bar or click + above" />
          ) : (
            workflowFiles.map((p) => (
              <div
                key={"wf-file-" + p.path}
                onClick={() => onAddWorkflowFile(p)}
                style={{
                  display: "flex", alignItems: "center", gap: 8,
                  padding: "5px 10px", borderRadius: 6, fontSize: 11,
                  color: "var(--fg)", cursor: "pointer",
                }}
                title={(p.description || p.path) + " · from " + p.workspace + " (click to add to chain)"}
              >
                <Workflow size={10} style={{ color: "var(--cyan)", flexShrink: 0 }} />
                <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {p.name}
                </span>
                <button
                  onClick={(e) => { e.stopPropagation(); onEditWorkflowFile(p); }}
                  style={{ background: "none", border: "none", color: "var(--purple)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.7 }}
                  title="Edit in popup"
                >
                  <Pencil size={9} />
                </button>
                <button
                  onClick={(e) => { e.stopPropagation(); onRunWorkflowFile(p); }}
                  style={{ background: "none", border: "none", color: "var(--green)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.7 }}
                  title="Run immediately"
                >
                  <Play size={9} />
                </button>
                <button
                  onClick={(e) => { e.stopPropagation(); onDeleteWorkflowFile(p); }}
                  style={{ background: "none", border: "none", color: "var(--fg-dim)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.2 }}
                  title="Delete workflow file"
                >
                  <Trash2 size={9} />
                </button>
              </div>
            ))
          )}
        </SidebarSection>

        <SidebarSection title="Prompts" icon={<FileText size={12} />} count={prompts.length} defaultOpen={prompts.length > 0} searchable>
          {(filter: string) => {
            const filtered = prompts.filter((p) =>
              !filter || p.Title.toLowerCase().includes(filter.toLowerCase())
            );
            return (
              <>
                {activeWorkspaceId && (
                  <button
                    onClick={onNewPrompt}
                    style={{
                      display: "flex", alignItems: "center", gap: 6,
                      padding: "5px 10px", marginBottom: 2,
                      fontSize: 11, color: "var(--fg-dim)",
                      background: "none", border: "none", borderRadius: 6,
                      cursor: "pointer", width: "100%",
                    }}
                    title="Draft a new prompt"
                  >
                    <Plus size={10} />
                    New prompt
                  </button>
                )}
                {filtered.length === 0 && (
                  <EmptyState text={prompts.length ? "No matches" : "No saved prompts"} />
                )}
                {filtered.map((p) => (
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
                      onClick={(e) => { e.stopPropagation(); onEditPrompt(p); }}
                      style={{ background: "none", border: "none", color: "var(--purple)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.7 }}
                      title="Edit in popup"
                    >
                      <Pencil size={9} />
                    </button>
                    <button
                      onClick={(e) => { e.stopPropagation(); onDeletePrompt(p.ID); }}
                      style={{ background: "none", border: "none", color: "var(--fg-dim)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.2 }}
                    >
                      <Trash2 size={9} />
                    </button>
                  </div>
                ))}
              </>
            );
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

        <SidebarSection
          title="Activity"
          icon={<Activity size={12} />}
          count={brainActivity.length}
          defaultOpen={brainActivity.some((e) => e.unread)}
        >
          {brainActivity.length === 0 ? (
            <EmptyState text="brain is quiet" />
          ) : (
            brainActivity.slice(0, 30).map((e) => <ActivityRow key={e.id} entry={e} />)
          )}
        </SidebarSection>
        </div>
      </div>
    </div>
  );
}

function ActivityRow({ entry }: { entry: BrainActivity }) {
  const sevColor =
    entry.severity === "error"
      ? "var(--red)"
      : entry.severity === "warn"
        ? "var(--yellow)"
        : "var(--cyan)";
  return (
    <div
      style={{
        display: "flex",
        gap: 8,
        padding: "5px 10px",
        fontSize: 11,
        color: "var(--fg)",
        alignItems: "flex-start",
      }}
      title={entry.detail}
    >
      <span
        style={{
          width: 5,
          height: 5,
          borderRadius: 999,
          background: sevColor,
          marginTop: 6,
          flexShrink: 0,
          boxShadow: entry.unread ? `0 0 5px ${sevColor}` : "none",
        }}
      />
      <span
        style={{
          flex: 1,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
          fontWeight: entry.unread ? 600 : 400,
          opacity: entry.unread ? 1 : 0.85,
        }}
      >
        {entry.title}
      </span>
      {entry.timestamp > 0 && (
        <span
          style={{
            fontSize: 9,
            color: "var(--fg-dim)",
            opacity: 0.7,
            fontVariantNumeric: "tabular-nums",
            flexShrink: 0,
          }}
          title={new Date(entry.timestamp).toLocaleString()}
        >
          {formatTime12(entry.timestamp)}
        </span>
      )}
    </div>
  );
}

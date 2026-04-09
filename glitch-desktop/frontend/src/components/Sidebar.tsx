import {
  Play,
  Bot,
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
import { useState, useEffect, useCallback } from "react";
import type { Workspace } from "@/lib/types";
import {
  ListWorkspaceDirectoriesDetailed,
  SetWorkspaceDirectoryEnabled,
  SetWorkspacePrimaryDirectory,
} from "../../wailsjs/go/main/App";

// DirectoryRow mirrors the JSON shape returned by
// ListWorkspaceDirectoriesDetailed: every directory associated with a
// workspace, including disabled ones, with the per-row enable + primary
// flags. The primary directory is the one git/gh tooling targets when
// the user asks a research question; additional directories are
// scanned by collectors for context but don't anchor research calls.
interface DirectoryRow {
  path: string;
  repo_name: string;
  enabled: boolean;
  primary: boolean;
}

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
  /** "workspace" | "global", normalized by the backend. */
  scope: string;
  /** Absolute file path (or directory for skills) so the editor can open it. */
  path: string;
  description: string;
  invoke: string;
}

export interface PromptEntry {
  ID: number;
  Title: string;
  Body: string;
  ModelSlug: string;
  /** Empty string = global, non-empty = workspace cwd. */
  CWD: string;
  UpdatedAt: number;
}

/** Tri-state segmented filter used by Prompts and Skills/Agents
 *  sections. We default to "workspace" because gl1tch is
 *  workspace-scoped by default — the user has to opt in to see global
 *  entries. */
type ScopeFilterValue = "workspace" | "global" | "all";

function ScopeFilter({
  value,
  onChange,
  workspaceCount,
  globalCount,
}: {
  value: ScopeFilterValue;
  onChange: (v: ScopeFilterValue) => void;
  workspaceCount: number;
  globalCount: number;
}) {
  const tabs: { id: ScopeFilterValue; label: string; count: number }[] = [
    { id: "workspace", label: "workspace", count: workspaceCount },
    { id: "global", label: "global", count: globalCount },
    { id: "all", label: "all", count: workspaceCount + globalCount },
  ];
  return (
    <div
      style={{
        display: "flex",
        gap: 2,
        marginBottom: 4,
        padding: 2,
        background: "var(--bg)",
        border: "1px solid var(--border)",
        borderRadius: 6,
      }}
    >
      {tabs.map((t) => {
        const active = value === t.id;
        return (
          <button
            key={t.id}
            onClick={() => onChange(t.id)}
            style={{
              flex: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: 4,
              padding: "3px 6px",
              fontSize: 9,
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: "0.04em",
              borderRadius: 4,
              border: "none",
              background: active ? "var(--bg-surface)" : "transparent",
              color: active ? "var(--cyan)" : "var(--fg-dim)",
              cursor: "pointer",
              fontFamily: "inherit",
            }}
            title={`Show ${t.label} entries (${t.count})`}
          >
            {t.label}
            {t.count > 0 && (
              <span
                style={{
                  fontSize: 8,
                  opacity: 0.7,
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                {t.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}

interface Props {
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
  directories: string[];
  agents: AgentEntry[];
  workflowFiles: WorkflowFileEntry[];
  prompts: PromptEntry[];
  selectedAgent: string | null;
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
  /** Open the EditorPopup with this skill or agent loaded. Global
   *  entries open read-only and force the user through "save as new". */
  onEditAgent: (a: AgentEntry) => void;
  onAddPrompt: (p: PromptEntry) => void;
  onDeletePrompt: (id: number) => void;
  /** Open the EditorPopup with this prompt loaded as a draft. */
  onEditPrompt: (p: PromptEntry) => void;
  /** Open the EditorPopup on a brand-new prompt draft. */
  onNewPrompt: () => void;
}

export function Sidebar({
  workspaces, activeWorkspaceId,
  directories, agents, workflowFiles, prompts, selectedAgent,
  onAddDirectory, onRemoveDirectory, onAddWorkflowFile,
  onRunWorkflowFile, onDeleteWorkflowFile, onEditWorkflowFile, onNewWorkflow,
  onAddAgent, onEditAgent,
  onAddPrompt, onDeletePrompt, onEditPrompt, onNewPrompt,
}: Props) {
  const totalWorkflows = workflowFiles.length;
  const activeWs = workspaces.find((w) => w.id === activeWorkspaceId);
  const contextLabel = activeWs?.title ?? "no workspace";
  const noWorkspace = !activeWorkspaceId;

  // Default both filter sections to "workspace" — users opt in to
  // global entries. State lives in the parent component so it
  // survives section collapse/expand.
  const [promptScope, setPromptScope] = useState<ScopeFilterValue>("workspace");
  const [agentScope, setAgentScope] = useState<ScopeFilterValue>("workspace");

  // Detailed directory rows including disabled entries. The parent's
  // `directories` prop only contains enabled paths (filtered at the
  // store layer so collector code never has to know about the toggle
  // column), so we fetch the full list separately to render the
  // per-directory pause/resume checkboxes. Refreshed on workspace
  // change and on the workspace:updated event the App emits after
  // every dir mutation.
  const [dirRows, setDirRows] = useState<DirectoryRow[]>([]);
  const refreshDirRows = useCallback(async () => {
    if (!activeWorkspaceId) {
      setDirRows([]);
      return;
    }
    try {
      const json = await ListWorkspaceDirectoriesDetailed(activeWorkspaceId);
      const parsed = JSON.parse(json);
      setDirRows(Array.isArray(parsed) ? parsed : []);
    } catch {
      setDirRows([]);
    }
  }, [activeWorkspaceId]);

  useEffect(() => {
    void refreshDirRows();
  }, [refreshDirRows, directories.length]);

  const handleToggleDir = useCallback(async (path: string, enabled: boolean) => {
    if (!activeWorkspaceId) return;
    // Optimistic update so the checkbox flips instantly; the pod
    // restart triggered by the backend will refresh the rest of the
    // sidebar via the workspace:updated event.
    setDirRows((rows) => rows.map((r) => (r.path === path ? { ...r, enabled } : r)));
    try {
      await SetWorkspaceDirectoryEnabled(activeWorkspaceId, path, enabled);
    } catch {
      // Revert on failure.
      void refreshDirRows();
    }
  }, [activeWorkspaceId, refreshDirRows]);

  const handleSetPrimary = useCallback(async (path: string) => {
    if (!activeWorkspaceId) return;
    // Optimistic: demote current primary, promote the target.
    setDirRows((rows) => rows.map((r) => ({ ...r, primary: r.path === path })));
    try {
      await SetWorkspacePrimaryDirectory(activeWorkspaceId, path);
    } catch {
      void refreshDirRows();
    }
  }, [activeWorkspaceId, refreshDirRows]);

  // Bucket prompts by scope. A prompt is "workspace" when its CWD
  // matches one of the active workspace's directories. CWD = "" is
  // always global. Computing this once per render keeps the filter
  // logic colocated with the component.
  const dirSet = new Set(directories);
  const promptIsWorkspace = (p: PromptEntry) => p.CWD !== "" && dirSet.has(p.CWD);
  const promptIsGlobal = (p: PromptEntry) => p.CWD === "";
  const promptWorkspaceCount = prompts.filter(promptIsWorkspace).length;
  const promptGlobalCount = prompts.filter(promptIsGlobal).length;

  // Bucket agents by scope. The backend already tagged each entry as
  // "workspace" or "global" via normalizeAgentScope, so this is just
  // a count.
  const agentWorkspaceCount = agents.filter((a) => a.scope === "workspace").length;
  const agentGlobalCount = agents.filter((a) => a.scope === "global").length;

  return (
    <div style={{
      height: "100%", background: "var(--bg-dark)", borderRight: "1px solid var(--border)",
      display: "flex", flexDirection: "column", width: 280, minWidth: 280, maxWidth: 280, overflow: "hidden",
    }}>
      {/* Active workspace header */}
      <div
        style={{
          padding: "14px 16px 10px",
          borderBottom: "1px solid var(--border)",
          background: "linear-gradient(180deg, rgba(125,207,255,0.04), transparent)",
        }}
      >
        <div
          style={{
            fontSize: 11,
            fontWeight: 700,
            textTransform: "uppercase",
            letterSpacing: "0.08em",
            color: noWorkspace ? "var(--fg-dim)" : "var(--cyan)",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
          title={contextLabel}
        >
          {contextLabel}
        </div>
      </div>

      <div style={{ flex: 1, overflowY: "auto", overflowX: "hidden" }}>
        <div
          style={{
            paddingTop: 2,
            opacity: noWorkspace ? 0.5 : 1,
            transition: "opacity 0.15s",
          }}
        >
        {/* Per-workspace directories. Each row has a checkbox that
            toggles the per-directory enable flag — when unchecked,
            the unified workspace collector skips it on the next
            tick (the change persists in SQLite and is applied via a
            pod restart in the SetWorkspaceDirectoryEnabled handler). */}
        <SidebarSection title="Directories" icon={<FolderOpen size={12} />} count={dirRows.length}>
          {dirRows.length === 0 ? (
            <EmptyState text={activeWorkspaceId ? `No directories in ${contextLabel}` : "Create a workspace first"} />
          ) : (
            dirRows.map((row) => (
              <div
                key={row.path}
                style={{
                  display: "flex", alignItems: "center", gap: 8,
                  padding: "5px 10px", borderRadius: 6, fontSize: 11,
                  color: row.enabled ? "var(--fg)" : "var(--fg-dim)",
                  opacity: row.enabled ? 1 : 0.55,
                  borderLeft: row.primary ? "2px solid var(--cyan)" : "2px solid transparent",
                }}
                onContextMenu={(e) => {
                  if (!row.primary && row.enabled) {
                    e.preventDefault();
                    void handleSetPrimary(row.path);
                  }
                }}
                title={row.primary
                  ? `${row.path} (primary — research loop targets this repo)`
                  : `${row.path} (right-click to set as primary)`}
              >
                <input
                  type="checkbox"
                  checked={row.enabled}
                  onChange={(e) => handleToggleDir(row.path, e.target.checked)}
                  style={{ flexShrink: 0, cursor: "pointer", margin: 0 }}
                  title={row.enabled ? `Pause ${row.path}` : `Resume ${row.path}`}
                />
                <Folder size={11} style={{ color: row.primary ? "var(--cyan)" : "var(--yellow)", flexShrink: 0 }} />
                <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {row.path.split("/").pop()}
                </span>
                {row.primary && (
                  <span style={{ fontSize: 9, color: "var(--cyan)", flexShrink: 0, fontWeight: 600, letterSpacing: "0.05em" }}>
                    PRIMARY
                  </span>
                )}
                {!row.primary && row.enabled && (
                  <button
                    onClick={() => void handleSetPrimary(row.path)}
                    style={{
                      background: "none", border: "none", color: "var(--fg-dim)",
                      cursor: "pointer", padding: 2, borderRadius: 4, display: "flex",
                      opacity: 0, fontSize: 9,
                    }}
                    className="glitch-dir-set-primary"
                    title="Set as primary"
                  >
                    ⬆
                  </button>
                )}
                <button
                  onClick={() => onRemoveDirectory(row.path)}
                  style={{ background: "none", border: "none", color: "var(--fg-dim)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.3 }}
                  title={`Remove ${row.path}`}
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
            // Apply scope filter first, then text search.
            const scoped = prompts.filter((p) => {
              if (promptScope === "all") return true;
              if (promptScope === "workspace") return promptIsWorkspace(p);
              return promptIsGlobal(p);
            });
            const filtered = scoped.filter((p) =>
              !filter || p.Title.toLowerCase().includes(filter.toLowerCase())
            );
            return (
              <>
                <ScopeFilter
                  value={promptScope}
                  onChange={setPromptScope}
                  workspaceCount={promptWorkspaceCount}
                  globalCount={promptGlobalCount}
                />
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
            const scoped = agents.filter((a) => {
              if (agentScope === "all") return true;
              return a.scope === agentScope;
            });
            const filtered = scoped.filter((a) =>
              !filter || a.name.toLowerCase().includes(filter.toLowerCase())
            );
            return (
              <>
                <ScopeFilter
                  value={agentScope}
                  onChange={setAgentScope}
                  workspaceCount={agentWorkspaceCount}
                  globalCount={agentGlobalCount}
                />
                {filtered.length === 0 && (
                  <EmptyState text={agents.length ? "No matches" : "Add directories to discover"} />
                )}
                {filtered.map((a) => (
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
                    <button
                      onClick={(e) => { e.stopPropagation(); onEditAgent(a); }}
                      style={{ background: "none", border: "none", color: "var(--purple)", cursor: "pointer", padding: 2, borderRadius: 4, display: "flex", opacity: 0.7 }}
                      title={a.scope === "global" ? "Open (read-only — use save as new to fork)" : "Edit in popup"}
                    >
                      <Pencil size={9} />
                    </button>
                  </div>
                ))}
              </>
            );
          }}
        </SidebarSection>
        </div>
      </div>
    </div>
  );
}

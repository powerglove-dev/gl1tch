import { useEffect, useCallback, useState, useRef } from "react";
import { Titlebar } from "./components/Titlebar";
import { Sidebar } from "./components/Sidebar";
import { WorkspaceTabs } from "./components/WorkspaceTabs";
import type { AgentEntry, WorkflowFileEntry, PromptEntry } from "./components/Sidebar";
import { ActivitySidebar } from "./components/ActivitySidebar";
import { MessageList } from "./components/MessageList";
import { ChatInput } from "./components/ChatInput";
import type { ProviderOption, ChainStep } from "./components/ChatInput";
import { EditorPopup } from "./components/EditorPopup";
import { PausePanel } from "./components/PausePanel";
import { parseAgentOutput } from "./lib/parseAgentOutput";
import { CollectorConfigModal } from "./components/collectors/CollectorConfigModal";
import { useToast } from "./components/Toast";

import { useChatStore } from "./lib/store";
import { useWailsEvent } from "./lib/wails";
import type { Workspace } from "./lib/types";
import {
  Ready,
  AskScoped,
  AskProvider,
  CreateWorkspace,
  ListWorkspaces,
  DeleteWorkspace,
  UpdateWorkspaceTitle,
  AddWorkspaceDirectory,
  RemoveWorkspaceDirectory,
  ListProviders,
  ListAgents,
  ListWorkflows,
  ListPrompts,
  SaveChatWorkflow,
  DeleteWorkflowFile,
  DeletePrompt,
  RunWorkflow,
  CreateDraft,
  CreateDraftFromTarget,
  RunChain,
  StepThroughStartFromChain,
  StepThroughAccept,
  StepThroughAbort,
  StepThroughEditOutput,
  StepThroughSave,
  AnswerClarification,
  Doctor,
  StopRun,
  SaveMessage,
  LoadMessages,
} from "../wailsjs/go/main/App";
import type { Message, Block, BrainActivity } from "./lib/types";

export function App() {
  const {
    state, active,
    addUserMessage, addUserChain, startAssistant, appendChunk, finishAssistant, streamError,
    applyBlockEvent,
    setStatus, toggleSidebar, toggleActivitySidebar, setMessages,
    setWorkspaces, setActiveWorkspace, addWorkspace, removeWorkspace, updateWorkspace,
    addBrainActivity, markBrainRead,
  } = useChatStore();

  const toast = useToast();

  const [providers, setProviders] = useState<ProviderOption[]>([]);
  const [agents, setAgents] = useState<AgentEntry[]>([]);
  const [workflowFiles, setWorkflowFiles] = useState<WorkflowFileEntry[]>([]);
  const [prompts, setPrompts] = useState<PromptEntry[]>([]);
  const [selectedProvider, setSelectedProvider] = useState("");
  const [selectedModel, setSelectedModel] = useState("");
  const [chain, setChain] = useState<ChainStep[]>([]);
  const [pendingClarify, setPendingClarify] = useState<{ workspace_id: string; run_id: string; question: string } | null>(null);
  // Active step-through sessions keyed by workspace id. Each entry holds
  // the session id plus (if paused) the step that's currently blocking
  // on a user decision. Sessions are per-workspace because StopRun and
  // the chat surface are already workspace-scoped — routing a pause to
  // the wrong workspace's chat would be worse than dropping it.
  // Each entry remembers the chain + resolved provider/model that started
  // the session so save-as and rewind-by-replay can reconstitute it. The
  // chain is cleared from the chat-input bar on send, so without this the
  // only copy of the original user intent would be in the frontend's
  // ephemeral state — and the session would be un-saveable mid-run.
  const [stepSessions, setStepSessions] = useState<
    Record<
      string,
      {
        sessionId: string;
        chain: ChainStep[];
        userText: string;
        provider: string;
        model: string;
        paused?: { stepId: string; output: string; stepIndex: number; stepTotal: number };
      }
    >
  >({});
  // The currently open EditorPopup, if any. Holds just the draft id —
  // the popup loads its own state from GetDraft so the parent doesn't
  // have to mirror the draft body, turn history, etc.
  const [openDraftId, setOpenDraftId] = useState<number | null>(null);
  // Structured collectors-config modal. The string is the optional
  // initial collector id to pre-select; null means "modal closed".
  // Lives at the App root for the same reason openDraftId does:
  // BrainIndicator emits the request, App owns the modal lifecycle.
  const [collectorConfig, setCollectorConfig] = useState<
    { initialCollectorId?: string } | null
  >(null);

  // Observer default model — what "observer" mode delegates to when a chain
  // step needs an actual executor. Persisted to localStorage so the user's
  // pick survives restarts. The user sets this from the picker (★ button).
  const [observerDefaultProvider, setObserverDefaultProvider] = useState<string>(
    () => localStorage.getItem("gl1tch:observerProvider") ?? "",
  );
  const [observerDefaultModel, setObserverDefaultModel] = useState<string>(
    () => localStorage.getItem("gl1tch:observerModel") ?? "",
  );

  // Tracks the most recently persisted (workspaceId, messageId) per workspace
  // so we don't re-save the same row on every effect re-run. Per-workspace so
  // a save in workspace A doesn't suppress a save in workspace B.
  const lastSavedRef = useRef<Map<string, string>>(new Map());
  // Workspaces we've already hydrated from the DB. Without this, switching
  // back to a workspace mid-stream would re-fire LoadMessages and clobber
  // the in-flight assistant message.
  const hydratedRef = useRef<Set<string>>(new Set());

  // ── Wails events ──────────────────────────────────────────────────────
  // All chat events carry a workspace_id so we can route them to the right
  // workspace's slice instead of whichever happens to be active when the
  // event arrives. The active view is just a derived slice off byWorkspace.
  useWailsEvent("chat:chunk", (data: unknown) => {
    const d = data as { workspace_id?: string; text?: string };
    if (d?.workspace_id && d?.text != null) appendChunk(d.workspace_id, d.text);
  });

  useWailsEvent("chat:done", (data: unknown) => {
    const d = data as { workspace_id?: string };
    if (d?.workspace_id) finishAssistant(d.workspace_id);
  });

  useWailsEvent("chat:error", (data: unknown) => {
    const d = data as { workspace_id?: string; message?: string };
    if (d?.workspace_id) streamError(d.workspace_id, d.message ?? "error");
  });

  // Structured block events from the chain runner's protocol splitter.
  // Each event is a {workspace_id, kind, block, attrs?, text?} map (see
  // encodeBlockEvent in glitch-desktop/app.go).
  useWailsEvent("chat:event", (data: unknown) => {
    if (!data || typeof data !== "object") return;
    const d = data as { workspace_id?: string };
    if (!d.workspace_id) return;
    applyBlockEvent(d.workspace_id, data as Parameters<typeof applyBlockEvent>[1]);
  });

  useWailsEvent("status:all", (data: unknown) => {
    const s = data as Record<string, boolean>;
    if (s) setStatus({ ollama: s.ollama ?? false, elasticsearch: s.elasticsearch ?? false, busd: s.busd ?? false });
  });

  // brain:status — live state of the brain indicator. Backend can emit
  // either the legacy ["idle"|"improving", detail] tuple or an object
  // {state, detail}. We coerce both to the new BrainState union; legacy
  // "improving" maps to "analyzing".
  useWailsEvent("brain:status", (data: unknown) => {
    let rawState = "";
    let detail = "";
    if (Array.isArray(data)) {
      rawState = (data[0] as string) ?? "";
      detail = (data[1] as string) ?? "";
    } else if (data && typeof data === "object") {
      const d = data as { state?: string; detail?: string };
      rawState = d.state ?? "";
      detail = d.detail ?? "";
    }
    const map: Record<string, "idle" | "collecting" | "analyzing" | "alert" | "error"> = {
      idle: "idle",
      collecting: "collecting",
      analyzing: "analyzing",
      improving: "analyzing", // legacy alias
      alert: "alert",
      error: "error",
    };
    setStatus({ brain: map[rawState] ?? "idle", brainDetail: detail });
  });

  // brain:activity — one entry for the activity stream. Sent by the
  // backend when the local-model triage loop produces a finding or when
  // the brain wants to drop a periodic check-in.
  useWailsEvent("brain:activity", (data: unknown) => {
    if (!data || typeof data !== "object") return;
    const d = data as Partial<BrainActivity>;
    if (!d.title) return;
    const entry: BrainActivity = {
      id: d.id ?? `brain-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      kind: d.kind === "alert" ? "alert" : "checkin",
      severity: d.severity === "warn" || d.severity === "error" ? d.severity : "info",
      title: d.title,
      detail: d.detail ?? "",
      source: d.source,
      timestamp: d.timestamp ?? Date.now(),
      unread: true,
    };
    addBrainActivity(entry);
  });

  useWailsEvent("workspace:updated", (data: unknown) => {
    if (typeof data === "string") {
      try { updateWorkspace(JSON.parse(data)); } catch {}
    }
    refreshSidebarData();
  });

  // Step-through lifecycle events. Routed per-workspace so a paused
  // session in workspace A doesn't flash an Accept button in the chat
  // of workspace B. "step_paused" stores the currently paused step;
  // "step_committed" clears it (the runner is moving on); "final" and
  // "error" drop the entire session entry (chat:done/chat:error already
  // flipped the streaming state).
  useWailsEvent("step-through:event", (data: unknown) => {
    if (!data || typeof data !== "object") return;
    const d = data as {
      workspace_id?: string;
      kind?: string;
      session_id?: string;
      step_id?: string;
      output?: string;
      step_index?: number;
      step_total?: number;
      error?: string;
    };
    const wsId = d.workspace_id;
    if (!wsId || !d.session_id) return;
    // The captured step "value" arrives raw from the executor — full of
    // gl1tch protocol noise (<<GLITCH_TEXT>> markers, gl1tch-stats JSON,
    // [wrote: …] acks, etc.). We run it through parseAgentOutput so the
    // pause panel shows the same clean body the chat surface renders;
    // otherwise the textarea is unreadable garbage.
    //
    // We also cap the editor draft to PAUSE_EDITOR_MAX_CHARS. A textarea
    // holding hundreds of KB of monospace text causes visible scroll
    // lag in Webkit (the user reported this on a grep dump). The full
    // value still lives in the session on the backend; if the user
    // hits Accept the runner uses the original. Edit & continue commits
    // whatever's in the textarea, which is the truncated view — that's
    // the honest tradeoff and matches "if you're going to edit a
    // hundred KB blob, the panel isn't the right place".
    const PAUSE_EDITOR_MAX_CHARS = 64 * 1024;
    let cleanedOutput = "";
    if (d.kind === "step_paused") {
      const parsed = parseAgentOutput(d.output ?? "").body;
      if (parsed.length > PAUSE_EDITOR_MAX_CHARS) {
        cleanedOutput =
          parsed.slice(0, PAUSE_EDITOR_MAX_CHARS) +
          `\n\n… [truncated ${parsed.length - PAUSE_EDITOR_MAX_CHARS} chars for display — Accept commits the full output]`;
      } else {
        cleanedOutput = parsed;
      }
    }

    setStepSessions((prev) => {
      const next = { ...prev };
      const existing = next[wsId];
      if (!existing || existing.sessionId !== d.session_id) {
        // Event for a session we don't track (e.g. HMR race) — drop it.
        return prev;
      }
      switch (d.kind) {
        case "step_paused":
          next[wsId] = {
            ...existing,
            paused: {
              stepId: d.step_id ?? "",
              output: cleanedOutput,
              stepIndex: d.step_index ?? 0,
              stepTotal: d.step_total ?? 0,
            },
          };
          break;
        case "step_committed":
          next[wsId] = { ...existing, paused: undefined };
          break;
        case "final":
        case "error":
          delete next[wsId];
          break;
      }
      return next;
    });
  });

  useWailsEvent("chat:clarify", (data: unknown) => {
    const d = data as { workspace_id?: string; run_id?: string; question?: string };
    if (d?.workspace_id && d?.run_id && d?.question) {
      setPendingClarify({ workspace_id: d.workspace_id, run_id: d.run_id, question: d.question });
      appendChunk(d.workspace_id, "\n\n**Clarification needed:** " + d.question + "\n");
    }
  });

  // ── Startup ───────────────────────────────────────────────────────────
  useEffect(() => {
    Ready();

    ListWorkspaces().then((json) => {
      try {
        const wss: Workspace[] = JSON.parse(json) ?? [];
        setWorkspaces(wss);
        if (wss.length > 0) setActiveWorkspace(wss[0].id);
      } catch {}
    });

    ListProviders().then((json) => {
      try { setProviders(JSON.parse(json) ?? []); } catch {}
    });

    ListPrompts().then((json) => {
      try { setPrompts(JSON.parse(json) ?? []); } catch {}
    });
  }, []);

  // Reload sidebar data (agents, workflow files, prompts).
  // Workflows are file-backed only as of the YAML unification — there's
  // no separate DB-backed list to fetch.
  const refreshSidebarData = useCallback(() => {
    if (state.activeWorkspaceId) {
      ListAgents(state.activeWorkspaceId).then((json) => {
        try { setAgents(JSON.parse(json) ?? []); } catch {}
      });
      ListWorkflows(state.activeWorkspaceId).then((json) => {
        try { setWorkflowFiles(JSON.parse(json) ?? []); } catch {}
      });
    } else {
      setAgents([]);
      setWorkflowFiles([]);
    }
    ListPrompts().then((json) => {
      try { setPrompts(JSON.parse(json) ?? []); } catch {}
    });
  }, [state.activeWorkspaceId]);

  // Load on workspace change
  useEffect(() => {
    refreshSidebarData();
  }, [state.activeWorkspaceId]);

  // ── Chat history persistence ──────────────────────────────────────────
  // Hydrate a workspace's messages from the DB the first time we see it.
  // Skipped on subsequent switches so the in-memory slice (which may have
  // an in-flight stream) is preserved. Workspaces with no rows still get
  // marked as hydrated so we don't refetch on every switch.
  useEffect(() => {
    const wsId = state.activeWorkspaceId;
    if (!wsId) return;
    if (hydratedRef.current.has(wsId)) return;
    hydratedRef.current.add(wsId);

    LoadMessages(wsId).then((json) => {
      try {
        const rows = JSON.parse(json) as Array<{
          id: string;
          role: string;
          blocks_json: string;
          timestamp: number;
        }>;
        const msgs: Message[] = (rows ?? []).map((r) => {
          let blocks: Block[] = [];
          try {
            const parsed = JSON.parse(r.blocks_json);
            if (Array.isArray(parsed)) blocks = parsed as Block[];
          } catch {}
          return {
            id: r.id,
            role: (r.role as Message["role"]) ?? "assistant",
            blocks,
            timestamp: r.timestamp,
          };
        });
        setMessages(wsId, msgs);
        const tail = msgs[msgs.length - 1];
        if (tail) lastSavedRef.current.set(wsId, tail.id);
      } catch {
        setMessages(wsId, []);
      }
    });
  }, [state.activeWorkspaceId, setMessages]);

  // Persist newly committed messages for *every* workspace that has unsaved
  // tail rows. We can't scope this to the active workspace because a stream
  // running in workspace A while the user is looking at workspace B would
  // never get its tail saved otherwise.
  useEffect(() => {
    for (const [wsId, slice] of Object.entries(state.byWorkspace)) {
      const last = slice.messages[slice.messages.length - 1];
      if (!last || last.streaming) continue;
      const seen = lastSavedRef.current.get(wsId);
      if (seen === last.id) continue;
      lastSavedRef.current.set(wsId, last.id);
      SaveMessage(
        wsId,
        JSON.stringify({
          id: last.id,
          role: last.role,
          blocks: last.blocks,
          timestamp: last.timestamp,
        }),
      );
    }
  }, [state.byWorkspace]);

  // Refresh when window regains focus (picks up new files/prompts)
  useEffect(() => {
    const onFocus = () => refreshSidebarData();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [refreshSidebarData]);

  // ── Keyboard shortcuts ────────────────────────────────────────────────
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.metaKey && e.key === "b") { e.preventDefault(); toggleSidebar(); }
      if (e.metaKey && e.key === "n") { e.preventDefault(); handleNewWorkspace(); }
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [toggleSidebar]);

  // ── Handlers ──────────────────────────────────────────────────────────

  const handleNewWorkspace = useCallback(() => {
    CreateWorkspace("New Chat").then((json) => {
      try {
        const ws: Workspace = JSON.parse(json);
        addWorkspace(ws);
        setActiveWorkspace(ws.id);
        // New workspaces have no DB rows; mark hydrated so the loader skips them.
        hydratedRef.current.add(ws.id);
      } catch {}
    });
  }, [addWorkspace, setActiveWorkspace]);

  const handleSwitchWorkspace = useCallback((id: string) => {
    setActiveWorkspace(id);
  }, [setActiveWorkspace]);

  const handleDeleteWorkspace = useCallback((id: string) => {
    DeleteWorkspace(id);
    removeWorkspace(id);
    hydratedRef.current.delete(id);
    lastSavedRef.current.delete(id);
  }, [removeWorkspace]);

  const handleRenameWorkspace = useCallback((id: string, title: string) => {
    UpdateWorkspaceTitle(id, title);
    const ws = (state.workspaces ?? []).find((w) => w.id === id);
    if (ws) updateWorkspace({ ...ws, title });
  }, [state.workspaces, updateWorkspace]);

  // ── Chain management ──────────────────────────────────────────────────

  const handleAddWorkflowFileToChain = useCallback((p: WorkflowFileEntry) => {
    setChain((prev) => [...prev, { type: "pipeline", path: p.path, label: p.name, description: p.description }]);
  }, []);

  const handleAddAgentToChain = useCallback((name: string) => {
    const agent = agents.find((a) => a.name === name);
    if (!agent) return;
    setChain((prev) => [...prev, {
      type: "agent", name: agent.name, label: agent.name,
      kind: agent.kind, invoke: agent.invoke,
    }]);
  }, [agents]);

  const handleAddPromptToChain = useCallback((p: PromptEntry) => {
    setChain((prev) => [...prev, { type: "prompt", id: p.ID, label: p.Title, body: p.Body }]);
  }, []);

  const handleRemoveChainStep = useCallback((index: number) => {
    setChain((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const handleUpdateChainStep = useCallback((index: number, step: ChainStep) => {
    setChain((prev) => prev.map((s, i) => (i === index ? step : s)));
  }, []);

  const handleClearChain = useCallback(() => setChain([]), []);

  // ── Workflow management ──────────────────────────────────────────────
  //
  // Workflows are file-backed (.workflow.yaml under
  // <workspace>/.glitch/workflows/) as of the YAML unification. There's
  // no DB intermediary anymore — saving the chain bar writes a YAML
  // file directly, and deleting removes the file.

  const handleSaveWorkflow = useCallback((name: string) => {
    if (!state.activeWorkspaceId || chain.length === 0) return;
    const stepsJSON = JSON.stringify(chain);
    // Resolve the provider/model that will get baked into the saved
    // YAML for any prompt step that doesn't override it. Same priority
    // as runChainNow: explicit picker → observer default → ollama.
    let provider = selectedProvider || observerDefaultProvider || "ollama";
    let model = selectedProvider ? selectedModel : observerDefaultModel;
    if (!model && provider === "ollama") {
      const ollama = providers.find((p) => p.id === "ollama");
      const def = ollama?.models.find((m) => m.default) ?? ollama?.models[0];
      model = def?.id ?? "";
    }
    SaveChatWorkflow(state.activeWorkspaceId!, name, stepsJSON, provider, model).then((result) => {
      // Backend returns {error: "..."} on failure or {path, name} on
      // success. Surface both via the toast — errors offer a retry
      // action, success is a quick confirmation so the user knows the
      // file actually landed.
      try {
        const parsed = JSON.parse(result);
        if (parsed?.error) {
          toast.error("Couldn't save workflow", {
            detail: parsed.error,
            actions: [{ label: "Retry", onClick: () => handleSaveWorkflow(name) }],
          });
          return;
        }
        toast.success(`Saved ${name}.workflow.yaml`, {
          detail: parsed?.path,
        });
      } catch {
        // Malformed payload — treat as a generic save failure rather
        // than swallowing it silently.
        toast.error("Couldn't save workflow", {
          detail: "Unexpected response from backend",
          actions: [{ label: "Retry", onClick: () => handleSaveWorkflow(name) }],
        });
      }
      refreshSidebarData();
    });
  }, [state.activeWorkspaceId, chain, refreshSidebarData, selectedProvider, selectedModel, observerDefaultProvider, observerDefaultModel, providers, toast]);

  // Run a workflow file directly (the sidebar's ▶ button). Calls the
  // backend's existing RunWorkflow path so the YAML gets executed in
  // the workspace's primary directory.
  const handleRunWorkflowFile = useCallback((p: WorkflowFileEntry) => {
    if (!state.activeWorkspaceId) return;
    const wsId = state.activeWorkspaceId;
    addUserMessage(wsId, `▶ ${p.name}`);
    startAssistant(wsId);
    RunWorkflow(p.path, "", wsId);
  }, [state.activeWorkspaceId, addUserMessage, startAssistant]);

  // Delete a workflow file from disk and refresh the sidebar list.
  // We optimistically remove from local state on success and surface
  // the path in the toast so the user knows exactly what got deleted.
  const handleDeleteWorkflowFile = useCallback((p: WorkflowFileEntry) => {
    DeleteWorkflowFile(p.path).then((err) => {
      if (err) {
        toast.error("Couldn't delete workflow", { detail: err });
        return;
      }
      setWorkflowFiles((prev) => prev.filter((wf) => wf.path !== p.path));
      toast.success(`Deleted ${p.name}.workflow.yaml`, { detail: p.path });
    });
  }, [toast]);

  // ── Editor popup handlers ────────────────────────────────────────────
  // Opening the popup is always "create or load a draft, then set
  // openDraftId". The popup component does the actual loading via
  // GetDraft so we don't need to mirror its state in the parent.

  const openEditor = useCallback((draftId: number) => {
    setOpenDraftId(draftId);
  }, []);

  const handleNewWorkflow = useCallback(() => {
    if (!state.activeWorkspaceId) return;
    // Empty body, empty title — the popup will prompt the user to
    // type a name and either chat-refine or hand-write the YAML.
    CreateDraft(state.activeWorkspaceId, "workflow", "", "").then((json) => {
      try {
        const d = JSON.parse(json);
        if (d?.error) {
          toast.error("Couldn't create draft", { detail: d.error });
          return;
        }
        if (d?.id) openEditor(d.id);
      } catch {}
    });
  }, [state.activeWorkspaceId, openEditor, toast]);

  const handleEditWorkflowFile = useCallback((p: WorkflowFileEntry) => {
    if (!state.activeWorkspaceId) return;
    CreateDraftFromTarget(state.activeWorkspaceId, "workflow", 0, p.path).then((json) => {
      try {
        const d = JSON.parse(json);
        if (d?.error) {
          toast.error("Couldn't open workflow", { detail: d.error });
          return;
        }
        if (d?.id) openEditor(d.id);
      } catch {}
    });
  }, [state.activeWorkspaceId, openEditor, toast]);

  const handleNewPrompt = useCallback(() => {
    if (!state.activeWorkspaceId) return;
    CreateDraft(state.activeWorkspaceId, "prompt", "", "").then((json) => {
      try {
        const d = JSON.parse(json);
        if (d?.error) {
          toast.error("Couldn't create draft", { detail: d.error });
          return;
        }
        if (d?.id) openEditor(d.id);
      } catch {}
    });
  }, [state.activeWorkspaceId, openEditor, toast]);

  const handleEditPrompt = useCallback((p: PromptEntry) => {
    if (!state.activeWorkspaceId) return;
    CreateDraftFromTarget(state.activeWorkspaceId, "prompt", p.ID, "").then((json) => {
      try {
        const d = JSON.parse(json);
        if (d?.error) {
          toast.error("Couldn't open prompt", { detail: d.error });
          return;
        }
        if (d?.id) openEditor(d.id);
      } catch {}
    });
  }, [state.activeWorkspaceId, openEditor, toast]);

  // Skills and agents share an edit handler — the kind comes from
  // the AgentEntry, the path from a.path, and the popup figures out
  // read_only based on whether the path is in workspace or global.
  const handleEditAgent = useCallback((a: AgentEntry) => {
    if (!state.activeWorkspaceId) return;
    CreateDraftFromTarget(state.activeWorkspaceId, a.kind, 0, a.path).then((json) => {
      try {
        const d = JSON.parse(json);
        if (d?.error) {
          toast.error("Couldn't open " + a.kind, { detail: d.error });
          return;
        }
        if (d?.id) openEditor(d.id);
      } catch {}
    });
  }, [state.activeWorkspaceId, openEditor, toast]);

  // Open the EditorPopup on the active workspace's collectors.yaml.
  // Same draft primitive as everything else — the popup loads it as
  // kind=collectors, refines via the chat strip, and saves back via
  // PromoteDraft (which pipes through WriteWorkspaceCollectorConfig
  // and triggers a pod restart).
  const handleEditCollectors = useCallback(() => {
    if (!state.activeWorkspaceId) return;
    CreateDraftFromTarget(state.activeWorkspaceId, "collectors", 0, "").then((json) => {
      try {
        const d = JSON.parse(json);
        if (d?.error) {
          toast.error("Couldn't open collectors", { detail: d.error });
          return;
        }
        if (d?.id) openEditor(d.id);
      } catch {}
    });
  }, [state.activeWorkspaceId, openEditor, toast]);

  const closeEditor = useCallback(() => {
    setOpenDraftId(null);
  }, []);

  // Open the structured collector-config modal. The pencil per-row in
  // the brain popover passes a collector id; the section-level "+ add"
  // button passes undefined and the modal lands on the first entry.
  const handleConfigureCollector = useCallback(
    (collectorId?: string) => {
      if (!state.activeWorkspaceId) return;
      setCollectorConfig({ initialCollectorId: collectorId });
    },
    [state.activeWorkspaceId],
  );

  const closeCollectorConfig = useCallback(() => {
    setCollectorConfig(null);
  }, []);

  // ── Send ──────────────────────────────────────────────────────────────

  // runChainNow is the canonical execution path for a chain + optional text.
  // Pinned to the workspace that was active at submit time so the run keeps
  // delivering events into the right slice even if the user switches away.
  const runChainNow = useCallback((chainToRun: ChainStep[], text: string) => {
    if (chainToRun.length === 0 && !text.trim()) return;
    const wsId = state.activeWorkspaceId ?? "";
    if (!wsId) return;

    // Resolve which provider will actually run prompt steps that don't set
    // their own override. Priority: picker → observer default → ollama.
    let resolvedDefaultProvider: string;
    let resolvedDefaultModel: string;

    if (selectedProvider) {
      resolvedDefaultProvider = selectedProvider;
      resolvedDefaultModel = selectedModel;
    } else if (observerDefaultProvider) {
      resolvedDefaultProvider = observerDefaultProvider;
      resolvedDefaultModel = observerDefaultModel;
    } else {
      resolvedDefaultProvider = "ollama";
      const ollama = providers.find((p) => p.id === "ollama");
      const def = ollama?.models.find((m) => m.default) ?? ollama?.models[0];
      resolvedDefaultModel = def?.id ?? "";
    }

    if (chainToRun.length > 0) {
      addUserChain(
        wsId,
        chainToRun.map((s) => ({
          label: s.label,
          kind: s.type,
          provider:
            s.type === "prompt"
              ? (s.executorOverride || resolvedDefaultProvider)
              : undefined,
          model:
            s.type === "prompt"
              ? (s.modelOverride || resolvedDefaultModel || undefined)
              : undefined,
        })),
        text.trim() || undefined,
      );
    } else {
      addUserMessage(wsId, text);
    }
    startAssistant(wsId);

    if (text.trim() === "/doctor" && chainToRun.length === 0) {
      Doctor(wsId);
      return;
    }

    if (chainToRun.length > 0) {
      // Step-through routing rule (see project_step_through_mode):
      // step-through isn't a mode, it's a property of the run. A chain
      // of 2+ steps (or with a trailing user text that makes it ≥2) is
      // consequential enough to pause between steps. A single step —
      // whether chain-built or user-typed — runs straight through via
      // the normal chain path so plain chat never pauses.
      const effectiveSteps =
        chainToRun.length + (text.trim() !== "" ? 1 : 0);
      if (effectiveSteps >= 2) {
        // Snapshot the chain + resolved provider/model so rewind-by-replay
        // and save-as can reconstitute the session even after the chat
        // bar's local chain state has been cleared.
        const capturedChain = chainToRun;
        const capturedText = text;
        const capturedProvider = resolvedDefaultProvider;
        const capturedModel = resolvedDefaultModel;
        StepThroughStartFromChain(
          JSON.stringify(capturedChain),
          capturedText,
          wsId,
          capturedProvider,
          capturedModel,
        ).then((result) => {
          try {
            const parsed = JSON.parse(result);
            if (parsed?.error) {
              streamError(wsId, parsed.error);
            } else if (parsed?.session_id) {
              setStepSessions((prev) => ({
                ...prev,
                [wsId]: {
                  sessionId: parsed.session_id,
                  chain: capturedChain,
                  userText: capturedText,
                  provider: capturedProvider,
                  model: capturedModel,
                },
              }));
            }
          } catch {
            streamError(wsId, "step-through: bad response from backend");
          }
        });
      } else {
        RunChain(
          JSON.stringify(chainToRun),
          text,
          wsId,
          resolvedDefaultProvider,
          resolvedDefaultModel,
        );
      }
      setChain([]);
    } else if (selectedProvider) {
      AskProvider(selectedProvider, selectedModel, text, wsId, "");
    } else {
      AskScoped(text, wsId);
    }
  }, [addUserMessage, addUserChain, startAssistant, streamError, selectedProvider, selectedModel, observerDefaultProvider, observerDefaultModel, providers, state.activeWorkspaceId]);

  // Accept / Abort handlers for the paused-step banner. These fire the
  // backend decision and optimistically clear the paused state — the
  // authoritative clear still happens when "step_committed" or
  // "final"/"error" arrives on the step-through:event topic.
  const handleStepAccept = useCallback(() => {
    const wsId = state.activeWorkspaceId;
    if (!wsId) return;
    const entry = stepSessions[wsId];
    if (!entry?.paused) return;
    StepThroughAccept(entry.sessionId).then(() => {
      setStepSessions((prev) => {
        const cur = prev[wsId];
        if (!cur || cur.sessionId !== entry.sessionId) return prev;
        return { ...prev, [wsId]: { ...cur, paused: undefined } };
      });
    });
  }, [state.activeWorkspaceId, stepSessions]);

  const handleStepAbort = useCallback(() => {
    const wsId = state.activeWorkspaceId;
    if (!wsId) return;
    const entry = stepSessions[wsId];
    if (!entry) return;
    StepThroughAbort(entry.sessionId).then(() => {
      setStepSessions((prev) => {
        const next = { ...prev };
        delete next[wsId];
        return next;
      });
    });
  }, [state.activeWorkspaceId, stepSessions]);

  // EditOutput — keep the step's execution result but replace its
  // captured "value" with whatever the user typed. Continues the run.
  // The draft now lives inside PausePanel (to avoid re-rendering App
  // on every keystroke); the panel hands the edited value back via
  // the onEditAndContinue callback when the user clicks the button.
  const handleStepEditOutput = useCallback(
    (editedValue: string) => {
      const wsId = state.activeWorkspaceId;
      if (!wsId) return;
      const entry = stepSessions[wsId];
      if (!entry?.paused) return;
      StepThroughEditOutput(entry.sessionId, editedValue).then(() => {
        setStepSessions((prev) => {
          const cur = prev[wsId];
          if (!cur || cur.sessionId !== entry.sessionId) return prev;
          return { ...prev, [wsId]: { ...cur, paused: undefined } };
        });
      });
    },
    [state.activeWorkspaceId, stepSessions],
  );

  // Save the currently running (or paused) session as a .workflow.yaml.
  // Reuses the StepThroughSave wails binding which re-serializes the
  // chain via ChainStepsToYAML on the backend. Produces identical output
  // to the chat-bar Save button — the two paths share the serializer.
  const handleStepSaveAs = useCallback(
    (name: string) => {
      const wsId = state.activeWorkspaceId;
      if (!wsId) return;
      const entry = stepSessions[wsId];
      if (!entry) return;
      StepThroughSave(entry.sessionId, name).then((result) => {
        try {
          const parsed = JSON.parse(result);
          if (parsed?.error) {
            toast.error("Couldn't save workflow", { detail: parsed.error });
            return;
          }
          toast.success(`Saved ${name}.workflow.yaml`, { detail: parsed?.path });
          refreshSidebarData();
        } catch {
          toast.error("Couldn't save workflow", {
            detail: "Unexpected response from backend",
          });
        }
      });
    },
    [state.activeWorkspaceId, stepSessions, toast, refreshSidebarData],
  );

  // Rewind-by-replay: abort the current session and re-run the captured
  // chain from step 0. Honest about token cost — we don't have a
  // checkpoint store wired for chain sessions, so every rewind replays
  // the whole thing. `overrideChain` lets callers substitute an edited
  // chain (used later when a prompt edit UI lands); pass undefined to
  // replay verbatim (pure retry).
  const handleStepRewind = useCallback(
    (overrideChain?: ChainStep[]) => {
      const wsId = state.activeWorkspaceId;
      if (!wsId) return;
      const entry = stepSessions[wsId];
      if (!entry) return;
      const nextChain = overrideChain ?? entry.chain;
      StepThroughAbort(entry.sessionId);
      setStepSessions((prev) => {
        const next = { ...prev };
        delete next[wsId];
        return next;
      });
      startAssistant(wsId);
      StepThroughStartFromChain(
        JSON.stringify(nextChain),
        entry.userText,
        wsId,
        entry.provider,
        entry.model,
      ).then((result) => {
        try {
          const parsed = JSON.parse(result);
          if (parsed?.error) {
            streamError(wsId, parsed.error);
            return;
          }
          if (parsed?.session_id) {
            setStepSessions((prev) => ({
              ...prev,
              [wsId]: {
                sessionId: parsed.session_id,
                chain: nextChain,
                userText: entry.userText,
                provider: entry.provider,
                model: entry.model,
              },
            }));
          }
        } catch {
          streamError(wsId, "step-through: bad response from backend");
        }
      });
    },
    [state.activeWorkspaceId, stepSessions, startAssistant, streamError],
  );

  const handleSend = useCallback(
    (text: string) => {
      // If there's a pending clarification for the active workspace, answer it.
      if (pendingClarify && pendingClarify.workspace_id === state.activeWorkspaceId) {
        addUserMessage(pendingClarify.workspace_id, text);
        AnswerClarification(pendingClarify.run_id, text);
        setPendingClarify(null);
        return;
      }
      runChainNow(chain, text);
    },
    [addUserMessage, pendingClarify, chain, runChainNow, state.activeWorkspaceId],
  );

  const handleStop = useCallback(() => {
    if (!state.activeWorkspaceId) return;
    StopRun(state.activeWorkspaceId);
  }, [state.activeWorkspaceId]);

  const handleSelectProvider = useCallback((providerId: string, modelId: string) => {
    setSelectedProvider(providerId);
    setSelectedModel(modelId);
  }, []);

  const handleSetObserverDefault = useCallback((providerId: string, modelId: string) => {
    setObserverDefaultProvider(providerId);
    setObserverDefaultModel(modelId);
    localStorage.setItem("gl1tch:observerProvider", providerId);
    localStorage.setItem("gl1tch:observerModel", modelId);
  }, []);

  const handleAction = useCallback(async () => {}, []);

  const handleDeletePrompt = useCallback((id: number) => {
    DeletePrompt(id);
    setPrompts((prev) => prev.filter((p) => p.ID !== id));
  }, []);

  const handleAddDirectory = useCallback(() => {
    if (state.activeWorkspaceId) AddWorkspaceDirectory(state.activeWorkspaceId);
  }, [state.activeWorkspaceId]);

  const handleRemoveDirectory = useCallback((dir: string) => {
    if (state.activeWorkspaceId) RemoveWorkspaceDirectory(state.activeWorkspaceId, dir);
  }, [state.activeWorkspaceId]);

  const activeWs = (state.workspaces ?? []).find((w) => w.id === state.activeWorkspaceId);
  const directories = activeWs?.directories ?? [];

  // doneModel/doneTokens were used by the now-deleted Statusbar. The
  // chat surface (DoneBlock + chain chip) already shows the same info,
  // so we no longer need to derive them here.

  // Derive selected agent from chain for sidebar highlight
  const selectedAgent = chain.find((s) => s.type === "agent")?.type === "agent"
    ? (chain.find((s) => s.type === "agent") as { name: string })?.name ?? null
    : null;

  const activeClarify =
    pendingClarify && pendingClarify.workspace_id === state.activeWorkspaceId ? pendingClarify : null;

  // Active step-through session for the current workspace, if any.
  // `paused` is only set while a step is blocking on Accept/Abort;
  // between-step streaming uses the normal chat surface.
  const activeStepSession = state.activeWorkspaceId
    ? stepSessions[state.activeWorkspaceId] ?? null
    : null;
  const activePaused = activeStepSession?.paused ?? null;

  return (
    <div style={{ height: "100%", display: "flex", flexDirection: "column", background: "var(--bg)" }}>
      <Titlebar
        sidebarOpen={state.sidebarOpen}
        onToggleSidebar={toggleSidebar}
        activitySidebarOpen={state.activitySidebarOpen}
        onToggleActivitySidebar={toggleActivitySidebar}
        brainState={state.status.brain}
        brainDetail={state.status.brainDetail}
        brainActivity={state.brainActivity}
        onMarkBrainRead={markBrainRead}
        activeWorkspaceId={state.activeWorkspaceId}
        activeWorkspaceTitle={activeWs?.title ?? ""}
        onEditCollectors={handleEditCollectors}
        onConfigureCollector={handleConfigureCollector}
      />

      <div style={{ flex: 1, display: "flex", overflow: "hidden" }}>
        {/* Workspace tab strip — always visible on the left edge */}
        <WorkspaceTabs
          workspaces={state.workspaces}
          activeWorkspaceId={state.activeWorkspaceId}
          onSwitch={handleSwitchWorkspace}
          onDelete={handleDeleteWorkspace}
          onRename={handleRenameWorkspace}
          onNew={handleNewWorkspace}
        />

        {state.sidebarOpen && (
          <Sidebar
            workspaces={state.workspaces}
            activeWorkspaceId={state.activeWorkspaceId}
            directories={directories}
            agents={agents}
            workflowFiles={workflowFiles}
            prompts={prompts}
            selectedAgent={selectedAgent}
            onAddDirectory={handleAddDirectory}
            onRemoveDirectory={handleRemoveDirectory}
            onAddWorkflowFile={handleAddWorkflowFileToChain}
            onRunWorkflowFile={handleRunWorkflowFile}
            onDeleteWorkflowFile={handleDeleteWorkflowFile}
            onEditWorkflowFile={handleEditWorkflowFile}
            onNewWorkflow={handleNewWorkflow}
            onAddAgent={handleAddAgentToChain}
            onEditAgent={handleEditAgent}
            onAddPrompt={handleAddPromptToChain}
            onDeletePrompt={handleDeletePrompt}
            onEditPrompt={handleEditPrompt}
            onNewPrompt={handleNewPrompt}
          />
        )}

        <div style={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0, background: "var(--bg)" }}>
          <MessageList
            messages={active.messages}
            onAction={handleAction}
            thinking={active.streaming ? active.thinking : ""}
          />
          {activePaused && state.activeWorkspaceId && (
            <PausePanel
              stepId={activePaused.stepId}
              stepIndex={activePaused.stepIndex}
              stepTotal={activePaused.stepTotal}
              originalOutput={activePaused.output}
              onAccept={handleStepAccept}
              onEditAndContinue={handleStepEditOutput}
              onRetry={() => handleStepRewind()}
              onAbort={handleStepAbort}
              onSaveAs={handleStepSaveAs}
            />
          )}
          <ChatInput
            disabled={false}
            streaming={active.streaming && !activeClarify}
            providers={providers}
            selectedProvider={selectedProvider}
            selectedModel={selectedModel}
            observerDefaultProvider={observerDefaultProvider}
            observerDefaultModel={observerDefaultModel}
            chain={chain}
            onSelectProvider={handleSelectProvider}
            onSetObserverDefault={handleSetObserverDefault}
            onUpdateChainStep={handleUpdateChainStep}
            onRemoveChainStep={handleRemoveChainStep}
            onClearChain={handleClearChain}
            onSaveWorkflow={handleSaveWorkflow}
            onSend={handleSend}
            onStop={handleStop}
          />
        </div>

        {/* Right-side activity sidebar — mirrors the left
            workspace sidebar's hide/show pattern. The brain
            popover used to render this list as a buried dropdown;
            it's now an ambient panel that's visible by default
            so users immediately see what the brain is up to
            without clicking the brain icon. */}
        {state.activitySidebarOpen && (
          <ActivitySidebar
            activity={state.brainActivity}
            onMarkRead={markBrainRead}
            onClose={toggleActivitySidebar}
          />
        )}
      </div>

      {/* Editor popup — modal overlay for editing prompts and
          workflows with the chat-driven refinement loop. The parent
          owns nothing except the open draft id; the popup loads its
          own state via GetDraft. */}
      {openDraftId != null && state.activeWorkspaceId && (
        <EditorPopup
          draftId={openDraftId}
          workspaceId={state.activeWorkspaceId}
          providers={providers}
          observerDefaultProvider={observerDefaultProvider}
          observerDefaultModel={observerDefaultModel}
          onSetObserverDefault={handleSetObserverDefault}
          onClose={closeEditor}
          onSaved={refreshSidebarData}
        />
      )}

      {/* Structured collector config modal — schema-driven editor
          for collectors.yaml. Lives at the root alongside EditorPopup
          so the brain popover can fire-and-forget. */}
      {collectorConfig && state.activeWorkspaceId && (
        <CollectorConfigModal
          workspaceId={state.activeWorkspaceId}
          initialCollectorId={collectorConfig.initialCollectorId}
          onClose={closeCollectorConfig}
          onSaved={refreshSidebarData}
          onEditRawYAML={handleEditCollectors}
        />
      )}
    </div>
  );
}

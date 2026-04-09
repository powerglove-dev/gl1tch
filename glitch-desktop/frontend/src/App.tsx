import { useEffect, useCallback, useState, useRef } from "react";
import { Titlebar } from "./components/Titlebar";
import { Sidebar } from "./components/Sidebar";
import { WorkspaceTabs } from "./components/WorkspaceTabs";
import type { AgentEntry, WorkflowFileEntry, PromptEntry } from "./components/Sidebar";
// ActivitySidebar import removed — component retired 2026-04-08.
// See the comment above the (former) render site in the JSX below.
import { MessageList } from "./components/MessageList";
import { ChatInput } from "./components/ChatInput";
import { ThreadSidePane } from "./components/ThreadSidePane";
import { SpawnThreadOnMessage } from "../wailsjs/go/main/App";
import type { ProviderOption, ChainStep } from "./components/ChatInput";
import { EditorPopup } from "./components/EditorPopup";
import { PausePanel } from "./components/PausePanel";
import { parseAgentOutput } from "./lib/parseAgentOutput";
import { CollectorConfigModal } from "./components/collectors/CollectorConfigModal";
import { IndexedDocsModal } from "./components/IndexedDocsModal";
import { ConfirmModal } from "./components/ConfirmModal";
import { useToast } from "./components/Toast";

import { useChatStore } from "./lib/store";
import { useWailsEvent } from "./lib/wails";
import type { Workspace } from "./lib/types";
import {
  Ready,
  Execute,
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
  CreateDraft,
  CreateDraftFromTarget,
  StepThroughAccept,
  StepThroughAbort,
  StepThroughEditOutput,
  StepThroughSave,
  AnswerClarification,
  Doctor,
  StopRun,
  SaveMessage,
  LoadMessages,
  SetActiveWorkspace,
} from "../wailsjs/go/main/App";
import type { Message, Block, BrainActivity } from "./lib/types";
import { appendAnalysisStream } from "./lib/analysisStreams";

export function App() {
  const {
    state, active,
    addUserMessage, addUserChain, startAssistant, appendChunk, injectAssistant, finishAssistant, streamError,
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
  // chat-threads side pane: holds the (threadID, parentMessageID) of
  // whichever message the user clicked the 💬 affordance on. null
  // when no thread is open. The pane is part of the chat itself —
  // every message is a potential thread anchor, and the pane lives
  // in the right column of the chat layout. The full parent Message
  // is looked up from active.messages by ID at render time so the
  // side pane stays in sync with any in-place updates the parent
  // receives (streaming chunks, etc.). id="" means the spawn round-
  // trip hasn't returned yet — the pane renders in a loading state
  // so the user sees instant feedback on the click instead of
  // waiting for the backend to construct a research host.
  const [activeThread, setActiveThread] = useState<{ id: string; parentID: string } | null>(null);

  // openThreadOnMessage is the click handler MessageList wires to its
  // 💬 affordance. It opens the side pane optimistically (the pane
  // mounts with an empty thread id and renders the parent message
  // immediately) and resolves the real thread id from the backend in
  // the background. The user sees the pane appear within one frame
  // instead of waiting for the research host to finish constructing.
  const openThreadOnMessage = useCallback(async (messageID: string) => {
    if (!state.activeWorkspaceId || !messageID) return;
    // Optimistic open: empty id signals "spawn in flight" to the
    // side pane, which then suppresses the GetThreadMessages fetch
    // until the real id arrives.
    setActiveThread({ id: "", parentID: messageID });
    try {
      const json = await SpawnThreadOnMessage(state.activeWorkspaceId, messageID);
      const thread = JSON.parse(json);
      if (thread && thread.id) {
        setActiveThread((prev) =>
          prev && prev.parentID === messageID
            ? { id: thread.id, parentID: messageID }
            : prev,
        );
      }
    } catch (err) {
      console.error("openThreadOnMessage failed", err);
    }
  }, [state.activeWorkspaceId]);
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
  // Indexed-docs drill-in modal state. Opened when the user clicks
  // "View all & analyze" under an indexing-kind activity row, or
  // an alert-kind row with referenced docs. Null means closed; the
  // object carries the target source, an optional lower-bound ms
  // timestamp for the doc query, and an optional pre-filled prompt
  // (used by alert-kind rows that carry the LLM judge's hook).
  const [indexedDocs, setIndexedDocs] = useState<
    {
      source: string;
      sinceMs?: number;
      initialPrompt?: string;
      // preselectRefs pre-ticks docs in the drill-in modal by sha
      // or url. Used by the per-card Analyze buttons on activity
      // items so one click opens the modal primed on exactly that
      // doc and runs analysis against it.
      preselectRefs?: string[];
    } | null
  >(null);

  // Delete confirmation modal — stores the pending destructive action so the
  // user must explicitly confirm before anything gets removed.
  const [confirmDelete, setConfirmDelete] = useState<{
    title: string;
    message: string;
    onConfirm: () => void;
  } | null>(null);

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

  // chat:inject carries a *complete* assistant message for the
  // given workspace. Unlike chat:chunk, it doesn't merge into the
  // active stream — it gets pushed as a standalone history item
  // so proactive daemon messages (attention classifier dropping a
  // high-attention artifact) can't race with a user-initiated
  // stream running in a different provider/workspace.
  useWailsEvent("chat:inject", (data: unknown) => {
    const d = data as { workspace_id?: string; text?: string } & Record<string, unknown>;
    if (!d?.workspace_id || d?.text == null) return;
    const { workspace_id, text, ...meta } = d;
    injectAssistant(workspace_id, text, meta);
  });

  useWailsEvent("chat:done", (data: unknown) => {
    const d = data as { workspace_id?: string };
    if (d?.workspace_id) finishAssistant(d.workspace_id);
  });

  useWailsEvent("chat:error", (data: unknown) => {
    const d = data as { workspace_id?: string; message?: string };
    if (d?.workspace_id) streamError(d.workspace_id, d.message ?? "error");
  });

  // Auto-open the thread side-pane when a research call completes
  // and produces a thread. Fired by Execute's freeform→research path.
  useWailsEvent("chat:thread_ready", (data: unknown) => {
    const d = data as { workspace_id?: string; thread_id?: string; parent_id?: string };
    if (!d?.workspace_id || !d?.thread_id) return;
    // Only open if we're still on the workspace that initiated the research.
    if (d.workspace_id === state.activeWorkspaceId) {
      setActiveThread({ id: d.thread_id, parentID: d.parent_id ?? "" });
    }
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
    // Three event kinds: alert (systray-fanout), checkin (low-noise
    // status), analysis (deep-analysis markdown from the opencode
    // loop). Anything else falls back to checkin.
    const kind: BrainActivity["kind"] =
      d.kind === "alert" ? "alert" :
      d.kind === "analysis" ? "analysis" :
      "checkin";
    const entry: BrainActivity = {
      id: d.id ?? `brain-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      kind,
      severity: d.severity === "warn" || d.severity === "error" ? d.severity : "info",
      title: d.title,
      detail: d.detail ?? "",
      source: d.source,
      repo: d.repo,
      event_type: d.event_type,
      event_key: d.event_key,
      model: d.model,
      duration_ms: d.duration_ms,
      workspace_id: d.workspace_id,
      timestamp: d.timestamp ?? Date.now(),
      unread: true,
      // Optional indexing/preview + refinement fields. Older backend
      // builds don't set these; the row renderer handles undefined
      // gracefully. Cast through `any` because the Wails payload
      // doesn't carry them on Partial<BrainActivity>.
      items: (d as any).items,
      delta: (d as any).delta,
      source_total: (d as any).source_total,
      last_seen_ms: (d as any).last_seen_ms,
      window_from_ms: (d as any).window_from_ms,
      parent_id: (d as any).parent_id,
    };
    addBrainActivity(entry);
  });

  // brain:analysis:stream — token/done/error events from an in-flight
  // ad-hoc activity analysis (AnalyzeActivityChunks on the backend).
  // We pipe each event into the store slice keyed by streamId; the
  // IndexedDocsModal subscribes to its own streamId and renders the
  // accumulated tokens as they arrive.
  useWailsEvent("brain:analysis:stream", (data: unknown) => {
    if (!data || typeof data !== "object") return;
    const d = data as {
      streamId?: string;
      kind?: "token" | "done" | "error";
      data?: string;
      error?: string;
    };
    if (!d.streamId || !d.kind) return;
    appendAnalysisStream(d.streamId, {
      streamId: d.streamId,
      kind: d.kind,
      data: d.data,
      error: d.error,
    });
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
        if (wss.length > 0) {
          setActiveWorkspace(wss[0].id);
          // Same scoping reason as handleSwitchWorkspace.
          void SetActiveWorkspace(wss[0].id);
        }
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
    // Tell the backend so the brain loop's activity refresh scopes
    // its ES query to this workspace's pod. Otherwise the activity
    // panel would surface deltas from every workspace pod, making
    // per-workspace poll intervals look like they're being ignored.
    void SetActiveWorkspace(id);
  }, [setActiveWorkspace]);

  const handleDeleteWorkspace = useCallback((id: string) => {
    const ws = (state.workspaces ?? []).find((w) => w.id === id);
    const name = ws?.title ?? "this workspace";
    setConfirmDelete({
      title: "Delete workspace?",
      message: `"${name}" and all its messages will be permanently deleted.`,
      onConfirm: () => {
        DeleteWorkspace(id);
        removeWorkspace(id);
        hydratedRef.current.delete(id);
        lastSavedRef.current.delete(id);
        setConfirmDelete(null);
      },
    });
  }, [state.workspaces, removeWorkspace]);

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

  // resolveProvider returns [provider, model] using the priority chain:
  // explicit picker → observer default → ollama fallback.
  const resolveProvider = useCallback((): [string, string] => {
    if (selectedProvider) return [selectedProvider, selectedModel];
    if (observerDefaultProvider) return [observerDefaultProvider, observerDefaultModel];
    const ollama = providers.find((p) => p.id === "ollama");
    const def = ollama?.models.find((m) => m.default) ?? ollama?.models[0];
    return ["ollama", def?.id ?? ""];
  }, [selectedProvider, selectedModel, observerDefaultProvider, observerDefaultModel, providers]);

  // Run a workflow file directly (the sidebar's ▶ button). Routes
  // through Execute with a single pipeline step so the workflow runs
  // in the workspace's primary directory with the user's provider.
  const handleRunWorkflowFile = useCallback((p: WorkflowFileEntry) => {
    if (!state.activeWorkspaceId) return;
    const wsId = state.activeWorkspaceId;
    const [provider, model] = resolveProvider();
    addUserMessage(wsId, `▶ ${p.name}`);
    startAssistant(wsId);
    Execute(JSON.stringify({
      workspace_id: wsId,
      input: "",
      steps: [{ type: "pipeline", label: p.name, path: p.path }],
      provider,
      model,
    }));
  }, [state.activeWorkspaceId, addUserMessage, startAssistant, resolveProvider]);

  // Delete a workflow file from disk and refresh the sidebar list.
  // We optimistically remove from local state on success and surface
  // the path in the toast so the user knows exactly what got deleted.
  const handleDeleteWorkflowFile = useCallback((p: WorkflowFileEntry) => {
    setConfirmDelete({
      title: "Delete workflow?",
      message: `"${p.name}.workflow.yaml" will be permanently deleted from disk.`,
      onConfirm: () => {
        DeleteWorkflowFile(p.path).then((err) => {
          if (err) {
            toast.error("Couldn't delete workflow", { detail: err });
            return;
          }
          setWorkflowFiles((prev) => prev.filter((wf) => wf.path !== p.path));
          toast.success(`Deleted ${p.name}.workflow.yaml`, { detail: p.path });
        });
        setConfirmDelete(null);
      },
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

  // runChainNow is the single execution path for everything the user
  // submits from the chat bar. One call to Execute; routing is server-side.
  const runChainNow = useCallback((chainToRun: ChainStep[], text: string) => {
    if (chainToRun.length === 0 && !text.trim()) return;
    const wsId = state.activeWorkspaceId ?? "";
    if (!wsId) return;

    const [provider, model] = resolveProvider();

    // UI: show the user's message / chain immediately.
    if (chainToRun.length > 0) {
      addUserChain(
        wsId,
        chainToRun.map((s) => ({
          label: s.label,
          kind: s.type,
          provider:
            s.type === "prompt"
              ? (s.executorOverride || provider)
              : undefined,
          model:
            s.type === "prompt"
              ? (s.modelOverride || model || undefined)
              : undefined,
        })),
        text.trim() || undefined,
      );
    } else {
      addUserMessage(wsId, text);
    }
    startAssistant(wsId);

    // /doctor stays its own path (health check, no LLM).
    if (text.trim() === "/doctor" && chainToRun.length === 0) {
      Doctor(wsId);
      return;
    }

    // Step-through: 2+ effective steps pause between each.
    const effectiveSteps = chainToRun.length + (text.trim() ? 1 : 0);
    const stepThrough = chainToRun.length > 0 && effectiveSteps >= 2;

    // Snapshot for step-through session tracking.
    const capturedChain = chainToRun;
    const capturedText = text;

    Execute(JSON.stringify({
      workspace_id: wsId,
      input: text,
      steps: chainToRun.length > 0 ? chainToRun : undefined,
      provider,
      model,
      step_through: stepThrough,
    })).then((result) => {
      if (!stepThrough) return;
      // Step-through returns {session_id} synchronously.
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
              provider,
              model,
            },
          }));
        }
      } catch {
        streamError(wsId, "step-through: bad response from backend");
      }
    });

    if (chainToRun.length > 0) setChain([]);
  }, [addUserMessage, addUserChain, startAssistant, streamError, resolveProvider, state.activeWorkspaceId]);

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
      Execute(JSON.stringify({
        workspace_id: wsId,
        input: entry.userText,
        steps: nextChain,
        provider: entry.provider,
        model: entry.model,
        step_through: true,
      })).then((result) => {
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
    const prompt = prompts.find((p) => p.ID === id);
    const name = prompt?.Title ?? "this prompt";
    setConfirmDelete({
      title: "Delete prompt?",
      message: `"${name}" will be permanently deleted.`,
      onConfirm: () => {
        DeletePrompt(id);
        setPrompts((prev) => prev.filter((p) => p.ID !== id));
        setConfirmDelete(null);
      },
    });
  }, [prompts]);

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

        <div style={{ flex: 1, display: "flex", flexDirection: "row", minWidth: 0, background: "var(--bg)" }}>
          {/* Main column: message list + chat input. Every message
              row carries a 💬 affordance (in MessageList) so the
              user can spawn a thread on any chat message. */}
          <div style={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0 }}>
            {/* When a thread is active it takes over the main chat
                area. The main message list and input are hidden (not
                unmounted — we keep them so streaming state isn't lost)
                and the thread renders in the same column. A back
                button returns to the main chat. */}
            {activeThread && state.activeWorkspaceId ? (
              <ThreadSidePane
                workspaceID={state.activeWorkspaceId}
                threadID={activeThread.id}
                parentMessageID={activeThread.parentID}
                parentMessage={active.messages.find((m) => m.id === activeThread.parentID)}
                onAction={handleAction}
                onClose={() => setActiveThread(null)}
                onSwitchThread={(id) => setActiveThread({ id, parentID: activeThread.parentID })}
                renderInput={(dispatchInThread, busy) => (
                  <ChatInput
                    disabled={false}
                    streaming={busy}
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
                    onSend={(text) => void dispatchInThread(text)}
                    onStop={() => {}}
                  />
                )}
              />
            ) : (
              <>
            <MessageList
              messages={active.messages}
              onAction={handleAction}
              thinking={active.streaming ? active.thinking : ""}
              onOpenThread={(id) => void openThreadOnMessage(id)}
              activeThreadParentID={activeThread?.parentID}
              workspaceID={state.activeWorkspaceId || ""}
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
              </>
            )}
          </div>
        </div>

        {/* Right-side activity sidebar — mirrors the left
            workspace sidebar's hide/show pattern. The brain
            popover used to render this list as a buried dropdown.
            As of 2026-04-08 it is RETIRED — all proactive signals
            from the daemon (high-attention analyses, classifier
            disabled nudges, triage alerts) now land as assistant
            messages in the chat pane via emitChatInject. The
            sidebar added a second UI surface the user had to look
            at separately, which defeated the "chat IS the
            interface" principle. The ActivitySidebar component
            and the store's brainActivity slice remain for the
            systray integration path, but they are no longer
            rendered. If the user ever wants an ambient feed back,
            flip activitySidebarOpen default to true in store.ts
            and re-enable the render below. */}
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

      {/* Indexed-docs drill-in modal — opened from the activity
          sidebar's indexing-kind rows. Streams opencode + qwen2.5:7b
          output over brain:analysis:stream as the user runs or
          refines analyses. */}
      {indexedDocs && (
        <IndexedDocsModal
          source={indexedDocs.source}
          sinceMs={indexedDocs.sinceMs}
          initialPrompt={indexedDocs.initialPrompt}
          preselectRefs={indexedDocs.preselectRefs}
          onClose={() => setIndexedDocs(null)}
        />
      )}

      {confirmDelete && (
        <ConfirmModal
          title={confirmDelete.title}
          message={confirmDelete.message}
          onConfirm={confirmDelete.onConfirm}
          onCancel={() => setConfirmDelete(null)}
        />
      )}
    </div>
  );
}

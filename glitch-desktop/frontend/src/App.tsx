import { useEffect, useCallback, useState } from "react";
import { Titlebar } from "./components/Titlebar";
import { Statusbar } from "./components/Statusbar";
import { Sidebar } from "./components/Sidebar";
import type { AgentEntry, WorkflowFileEntry, PromptEntry, WorkflowEntry } from "./components/Sidebar";
import { MessageList } from "./components/MessageList";
import { ChatInput } from "./components/ChatInput";
import type { ProviderOption, ChainStep } from "./components/ChatInput";

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
  ListChatWorkflows,
  SaveChatWorkflow,
  DeleteChatWorkflow,
  DeletePrompt,
  RunChain,
  AnswerClarification,
  Doctor,
} from "../wailsjs/go/main/App";

export function App() {
  const {
    state,
    addUserMessage, startAssistant, appendChunk, finishAssistant, streamError,
    setStatus, toggleSidebar, clearMessages,
    setWorkspaces, setActiveWorkspace, addWorkspace, removeWorkspace, updateWorkspace,
  } = useChatStore();

  const [providers, setProviders] = useState<ProviderOption[]>([]);
  const [agents, setAgents] = useState<AgentEntry[]>([]);
  const [workflowFiles, setWorkflowFiles] = useState<WorkflowFileEntry[]>([]);
  const [prompts, setPrompts] = useState<PromptEntry[]>([]);
  const [workflows, setWorkflows] = useState<WorkflowEntry[]>([]);
  const [selectedProvider, setSelectedProvider] = useState("");
  const [selectedModel, setSelectedModel] = useState("");
  const [chain, setChain] = useState<ChainStep[]>([]);
  const [pendingClarify, setPendingClarify] = useState<{ run_id: string; question: string } | null>(null);

  // ── Wails events ──────────────────────────────────────────────────────
  useWailsEvent("chat:chunk", (text: string) => appendChunk(text));
  useWailsEvent("chat:done", () => finishAssistant());
  useWailsEvent("chat:error", (msg: string) => streamError(msg));

  useWailsEvent("status:all", (data: unknown) => {
    const s = data as Record<string, boolean>;
    if (s) setStatus({ ollama: s.ollama ?? false, elasticsearch: s.elasticsearch ?? false, busd: s.busd ?? false });
  });

  useWailsEvent("brain:status", (data: unknown) => {
    if (Array.isArray(data)) {
      setStatus({ brain: (data[0] as "idle" | "improving") ?? "idle", brainDetail: (data[1] as string) ?? "" });
    }
  });

  useWailsEvent("workspace:updated", (data: unknown) => {
    if (typeof data === "string") {
      try { updateWorkspace(JSON.parse(data)); } catch {}
    }
    refreshSidebarData();
  });

  useWailsEvent("chat:clarify", (data: unknown) => {
    const d = data as { run_id: string; question: string };
    if (d?.run_id && d?.question) {
      setPendingClarify(d);
      appendChunk("\n\n**Clarification needed:** " + d.question + "\n");
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

  // Reload sidebar data (agents, workflow files, prompts, saved workflows)
  const refreshSidebarData = useCallback(() => {
    if (state.activeWorkspaceId) {
      ListAgents(state.activeWorkspaceId).then((json) => {
        try { setAgents(JSON.parse(json) ?? []); } catch {}
      });
      ListWorkflows(state.activeWorkspaceId).then((json) => {
        try { setWorkflowFiles(JSON.parse(json) ?? []); } catch {}
      });
      ListChatWorkflows(state.activeWorkspaceId).then((json) => {
        try { setWorkflows(JSON.parse(json) ?? []); } catch {}
      });
    } else {
      setAgents([]);
      setWorkflowFiles([]);
      setWorkflows([]);
    }
    ListPrompts().then((json) => {
      try { setPrompts(JSON.parse(json) ?? []); } catch {}
    });
  }, [state.activeWorkspaceId]);

  // Load on workspace change
  useEffect(() => {
    refreshSidebarData();
  }, [state.activeWorkspaceId]);

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
        clearMessages();
      } catch {}
    });
  }, [addWorkspace, setActiveWorkspace, clearMessages]);

  const handleSwitchWorkspace = useCallback((id: string) => {
    setActiveWorkspace(id);
    clearMessages();
  }, [setActiveWorkspace, clearMessages]);

  const handleDeleteWorkspace = useCallback((id: string) => {
    DeleteWorkspace(id);
    removeWorkspace(id);
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

  const handleSaveWorkflow = useCallback((name: string) => {
    if (!state.activeWorkspaceId || chain.length === 0) return;
    const stepsJSON = JSON.stringify(chain);
    SaveChatWorkflow(state.activeWorkspaceId, name, stepsJSON).then(() => {
      refreshSidebarData();
    });
  }, [state.activeWorkspaceId, chain, refreshSidebarData]);

  const handleLoadWorkflow = useCallback((w: WorkflowEntry) => {
    try {
      const steps = JSON.parse(w.StepsJSON) as ChainStep[];
      setChain(steps);
    } catch {}
  }, []);

  // Run immediately: load the workflow into the chain and trigger send with no extra text.
  // We use a microtask so the chain state is committed before send reads it.
  const handleRunWorkflow = useCallback((w: WorkflowEntry) => {
    try {
      const steps = JSON.parse(w.StepsJSON) as ChainStep[];
      setChain(steps);
      // Defer send so the chain state is in place
      setTimeout(() => {
        // We can't call handleSend here directly because of stale closure;
        // instead, simulate by dispatching to the existing flow.
        // Use a ref-based approach: just call the send logic inline.
        runChainNow(steps, "");
      }, 0);
    } catch {}
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleDeleteWorkflow = useCallback((id: number) => {
    DeleteChatWorkflow(id).then(() => {
      setWorkflows((prev) => prev.filter((w) => w.ID !== id));
    });
  }, []);

  // ── Send ──────────────────────────────────────────────────────────────

  // runChainNow is the canonical execution path for a chain + optional text.
  // Used by both manual send (handleSend) and auto-run (handleRunWorkflow).
  // For chains: calls backend RunChain which runs each step sequentially with
  // its own provider, threading output between steps.
  const runChainNow = useCallback((chainToRun: ChainStep[], text: string) => {
    const chainLabels = chainToRun.map((s) => `[${s.label}]`).join(" -> ");
    const displayText = chainLabels ? (text ? `${chainLabels} -> ${text}` : chainLabels) : text;
    if (!displayText) return;

    addUserMessage(displayText);
    startAssistant();

    if (text.trim() === "/doctor" && chainToRun.length === 0) {
      Doctor();
      return;
    }

    if (chainToRun.length > 0) {
      // Default provider for steps that don't override. Falls back to
      // "github-copilot" so the chain still runs even when the picker is
      // on "observer" (which only handles ES queries, not arbitrary prompts).
      const defaultProvider = selectedProvider || "github-copilot";
      const defaultModel = selectedModel;
      RunChain(
        JSON.stringify(chainToRun),
        text,
        state.activeWorkspaceId ?? "",
        defaultProvider,
        defaultModel,
      );
      setChain([]);
    } else if (selectedProvider) {
      AskProvider(selectedProvider, selectedModel, text, state.activeWorkspaceId ?? "", "");
    } else {
      AskScoped(text, state.activeWorkspaceId ?? "");
    }
  }, [addUserMessage, startAssistant, selectedProvider, selectedModel, state.activeWorkspaceId]);

  const handleSend = useCallback(
    (text: string) => {
      // If there's a pending clarification, answer it instead
      if (pendingClarify) {
        addUserMessage(text);
        AnswerClarification(pendingClarify.run_id, text);
        setPendingClarify(null);
        return;
      }
      runChainNow(chain, text);
    },
    [addUserMessage, pendingClarify, chain, runChainNow],
  );

  const handleSelectProvider = useCallback((providerId: string, modelId: string) => {
    setSelectedProvider(providerId);
    setSelectedModel(modelId);
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

  const lastDone = state.messages.flatMap((m) => m.blocks).filter((b) => b.type === "done").at(-1);
  const doneModel = lastDone?.type === "done" ? lastDone.model : undefined;
  const doneTokens = lastDone?.type === "done" ? lastDone.tokens : undefined;

  // Derive selected agent from chain for sidebar highlight
  const selectedAgent = chain.find((s) => s.type === "agent")?.type === "agent"
    ? (chain.find((s) => s.type === "agent") as { name: string })?.name ?? null
    : null;

  return (
    <div style={{ height: "100%", display: "flex", flexDirection: "column", background: "var(--bg)" }}>
      <Titlebar sidebarOpen={state.sidebarOpen} onToggleSidebar={toggleSidebar} />

      <div style={{ flex: 1, display: "flex", overflow: "hidden" }}>
        {state.sidebarOpen && (
          <Sidebar
            onNewWorkspace={handleNewWorkspace}
            workspaces={state.workspaces}
            activeWorkspaceId={state.activeWorkspaceId}
            onSwitchWorkspace={handleSwitchWorkspace}
            onDeleteWorkspace={handleDeleteWorkspace}
            onRenameWorkspace={handleRenameWorkspace}
            directories={directories}
            agents={agents}
            workflowFiles={workflowFiles}
            prompts={prompts}
            workflows={workflows}
            selectedAgent={selectedAgent}
            onAddDirectory={handleAddDirectory}
            onRemoveDirectory={handleRemoveDirectory}
            onAddWorkflowFile={handleAddWorkflowFileToChain}
            onAddAgent={handleAddAgentToChain}
            onAddPrompt={handleAddPromptToChain}
            onLoadWorkflow={handleLoadWorkflow}
            onRunWorkflow={handleRunWorkflow}
            onDeleteWorkflow={handleDeleteWorkflow}
            onDeletePrompt={handleDeletePrompt}
          />
        )}

        <div style={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0, background: "var(--bg)" }}>
          <MessageList messages={state.messages} onAction={handleAction} />
          <ChatInput
            disabled={state.streaming && !pendingClarify}
            providers={providers}
            selectedProvider={selectedProvider}
            selectedModel={selectedModel}
            chain={chain}
            onSelectProvider={handleSelectProvider}
            onUpdateChainStep={handleUpdateChainStep}
            onRemoveChainStep={handleRemoveChainStep}
            onClearChain={handleClearChain}
            onSaveWorkflow={handleSaveWorkflow}
            onSend={handleSend}
          />
        </div>
      </div>

      <Statusbar status={state.status} model={selectedProvider || doneModel} tokens={doneTokens} />
    </div>
  );
}

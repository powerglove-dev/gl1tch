import { useEffect, useCallback, useState } from "react";
import { Titlebar } from "./components/Titlebar";
import { Statusbar } from "./components/Statusbar";
import { Sidebar } from "./components/Sidebar";
import type { AgentEntry, PipelineEntry } from "./components/Sidebar";
import { MessageList } from "./components/MessageList";
import { ChatInput } from "./components/ChatInput";
import type { ProviderOption } from "./components/ChatInput";
// AgentOption re-exported for convenience

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
  ListPipelines,
  RunPipeline,
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
  const [pipelines, setPipelines] = useState<PipelineEntry[]>([]);
  const [selectedProvider, setSelectedProvider] = useState("");
  const [selectedModel, setSelectedModel] = useState("");
  const [selectedAgent, setSelectedAgent] = useState("");

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
  }, []);

  // Load agents and pipelines when active workspace changes
  useEffect(() => {
    if (state.activeWorkspaceId) {
      ListAgents(state.activeWorkspaceId).then((json) => {
        try { setAgents(JSON.parse(json) ?? []); } catch {}
      });
      ListPipelines(state.activeWorkspaceId).then((json) => {
        try { setPipelines(JSON.parse(json) ?? []); } catch {}
      });
    } else {
      setAgents([]);
      setPipelines([]);
    }
  }, [state.activeWorkspaceId]);

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

  const handleSend = useCallback(
    (text: string) => {
      addUserMessage(text);
      startAssistant();

      if (text.trim() === "/doctor") {
        Doctor();
      } else if (selectedProvider) {
        // Direct provider — with glitch context + optional agent
        const agentPath = selectedAgent
          ? agents.find((a) => a.name === selectedAgent)?.invoke ?? ""
          : "";
        AskProvider(selectedProvider, selectedModel, text, state.activeWorkspaceId ?? "", agentPath);
      } else {
        // Observer — scoped to workspace
        AskScoped(text, state.activeWorkspaceId ?? "");
      }
    },
    [addUserMessage, startAssistant, selectedProvider, selectedModel, selectedAgent, agents, state.activeWorkspaceId],
  );

  const handleSelectProvider = useCallback((providerId: string, modelId: string) => {
    setSelectedProvider(providerId);
    setSelectedModel(modelId);
  }, []);

  const handleAction = useCallback(async () => {}, []);

  const handleRunPipeline = useCallback((path: string) => {
    const name = path.split("/").pop()?.replace(".pipeline.yaml", "") ?? "pipeline";
    addUserMessage(`/run ${name}`);
    startAssistant();
    RunPipeline(path, "");
  }, [addUserMessage, startAssistant]);

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

  return (
    <div style={{ height: "100%", display: "flex", flexDirection: "column", background: "var(--bg)" }}>
      <Titlebar status={state.status} sidebarOpen={state.sidebarOpen} onToggleSidebar={toggleSidebar} />

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
            pipelines={pipelines}
            selectedAgent={selectedAgent}
            onAddDirectory={handleAddDirectory}
            onRemoveDirectory={handleRemoveDirectory}
            onRunPipeline={handleRunPipeline}
            onSelectAgent={setSelectedAgent}
          />
        )}

        <div style={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0, background: "var(--bg)" }}>
          <MessageList messages={state.messages} onAction={handleAction} />
          <ChatInput
            disabled={state.streaming}
            providers={providers}
            selectedProvider={selectedProvider}
            selectedModel={selectedModel}
            selectedAgent={selectedAgent}
            onSelectProvider={handleSelectProvider}
            onClearAgent={() => setSelectedAgent("")}
            onSend={handleSend}
          />
        </div>
      </div>

      <Statusbar status={state.status} model={selectedProvider || doneModel} tokens={doneTokens} />
    </div>
  );
}

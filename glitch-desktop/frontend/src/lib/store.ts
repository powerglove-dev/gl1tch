import { useReducer, useCallback } from "react";
import type { Message, Block, SystemStatus, Workspace } from "./types";

interface ChatState {
  messages: Message[];
  streaming: boolean;
  status: SystemStatus;
  sidebarOpen: boolean;
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
}

type Action =
  | { type: "ADD_USER_MESSAGE"; text: string }
  | { type: "START_ASSISTANT" }
  | { type: "APPEND_CHUNK"; text: string }
  | { type: "APPEND_BLOCK"; block: Block }
  | { type: "FINISH_ASSISTANT"; meta?: { model?: string; tokens?: number; latency_ms?: number } }
  | { type: "STREAM_ERROR"; message: string }
  | { type: "SET_STATUS"; status: Partial<SystemStatus> }
  | { type: "TOGGLE_SIDEBAR" }
  | { type: "CLEAR_MESSAGES" }
  | { type: "SET_MESSAGES"; messages: Message[] }
  | { type: "SET_WORKSPACES"; workspaces: Workspace[] }
  | { type: "SET_ACTIVE_WORKSPACE"; id: string | null }
  | { type: "ADD_WORKSPACE"; workspace: Workspace }
  | { type: "REMOVE_WORKSPACE"; id: string }
  | { type: "UPDATE_WORKSPACE"; workspace: Workspace };

let nextId = 0;
function makeId() {
  return `msg-${Date.now()}-${nextId++}`;
}

const initialState: ChatState = {
  messages: [],
  streaming: false,
  status: { ollama: false, elasticsearch: false, busd: false, brain: "idle", brainDetail: "" },
  sidebarOpen: true,
  workspaces: [],
  activeWorkspaceId: null,
};

function reducer(state: ChatState, action: Action): ChatState {
  switch (action.type) {
    case "ADD_USER_MESSAGE":
      return {
        ...state,
        messages: [
          ...state.messages,
          {
            id: makeId(),
            role: "user",
            blocks: [{ type: "text", content: action.text }],
            timestamp: Date.now(),
          },
        ],
      };

    case "START_ASSISTANT":
      return {
        ...state,
        streaming: true,
        messages: [
          ...state.messages,
          {
            id: makeId(),
            role: "assistant",
            blocks: [],
            timestamp: Date.now(),
            streaming: true,
          },
        ],
      };

    case "APPEND_CHUNK": {
      const msgs = [...state.messages];
      const last = { ...msgs[msgs.length - 1] };
      const blocks = [...last.blocks];
      const lastBlock = blocks[blocks.length - 1];
      if (lastBlock && lastBlock.type === "text") {
        blocks[blocks.length - 1] = { ...lastBlock, content: lastBlock.content + action.text };
      } else {
        blocks.push({ type: "text", content: action.text });
      }
      last.blocks = blocks;
      msgs[msgs.length - 1] = last;
      return { ...state, messages: msgs };
    }

    case "APPEND_BLOCK": {
      const msgs = [...state.messages];
      const last = { ...msgs[msgs.length - 1] };
      last.blocks = [...last.blocks, action.block];
      msgs[msgs.length - 1] = last;
      return { ...state, messages: msgs };
    }

    case "FINISH_ASSISTANT": {
      const msgs = [...state.messages];
      const last = { ...msgs[msgs.length - 1] };
      last.streaming = false;
      if (action.meta) {
        last.blocks = [...last.blocks, { type: "done" as const, ...action.meta }];
      }
      msgs[msgs.length - 1] = last;
      return { ...state, streaming: false, messages: msgs };
    }

    case "STREAM_ERROR": {
      const msgs = [...state.messages];
      const last = { ...msgs[msgs.length - 1] };
      last.streaming = false;
      last.blocks = [...last.blocks, { type: "error", message: action.message }];
      msgs[msgs.length - 1] = last;
      return { ...state, streaming: false, messages: msgs };
    }

    case "SET_STATUS":
      return { ...state, status: { ...state.status, ...action.status } };

    case "TOGGLE_SIDEBAR":
      return { ...state, sidebarOpen: !state.sidebarOpen };

    case "CLEAR_MESSAGES":
      return { ...state, messages: [] };

    case "SET_MESSAGES":
      return { ...state, messages: action.messages };

    case "SET_WORKSPACES":
      return { ...state, workspaces: action.workspaces };

    case "SET_ACTIVE_WORKSPACE":
      return { ...state, activeWorkspaceId: action.id };

    case "ADD_WORKSPACE":
      return { ...state, workspaces: [action.workspace, ...state.workspaces] };

    case "REMOVE_WORKSPACE":
      return {
        ...state,
        workspaces: state.workspaces.filter((w) => w.id !== action.id),
        activeWorkspaceId: state.activeWorkspaceId === action.id ? null : state.activeWorkspaceId,
        messages: state.activeWorkspaceId === action.id ? [] : state.messages,
      };

    case "UPDATE_WORKSPACE":
      return {
        ...state,
        workspaces: state.workspaces.map((w) =>
          w.id === action.workspace.id ? action.workspace : w,
        ),
      };

    default:
      return state;
  }
}

export function useChatStore() {
  const [state, dispatch] = useReducer(reducer, initialState);

  return {
    state,
    addUserMessage: useCallback((text: string) => dispatch({ type: "ADD_USER_MESSAGE", text }), []),
    startAssistant: useCallback(() => dispatch({ type: "START_ASSISTANT" }), []),
    appendChunk: useCallback((text: string) => dispatch({ type: "APPEND_CHUNK", text }), []),
    appendBlock: useCallback((block: Block) => dispatch({ type: "APPEND_BLOCK", block }), []),
    finishAssistant: useCallback((meta?: { model?: string; tokens?: number; latency_ms?: number }) => dispatch({ type: "FINISH_ASSISTANT", meta }), []),
    streamError: useCallback((message: string) => dispatch({ type: "STREAM_ERROR", message }), []),
    setStatus: useCallback((status: Partial<SystemStatus>) => dispatch({ type: "SET_STATUS", status }), []),
    toggleSidebar: useCallback(() => dispatch({ type: "TOGGLE_SIDEBAR" }), []),
    clearMessages: useCallback(() => dispatch({ type: "CLEAR_MESSAGES" }), []),
    setMessages: useCallback((messages: Message[]) => dispatch({ type: "SET_MESSAGES", messages }), []),
    setWorkspaces: useCallback((workspaces: Workspace[]) => dispatch({ type: "SET_WORKSPACES", workspaces }), []),
    setActiveWorkspace: useCallback((id: string | null) => dispatch({ type: "SET_ACTIVE_WORKSPACE", id }), []),
    addWorkspace: useCallback((workspace: Workspace) => dispatch({ type: "ADD_WORKSPACE", workspace }), []),
    removeWorkspace: useCallback((id: string) => dispatch({ type: "REMOVE_WORKSPACE", id }), []),
    updateWorkspace: useCallback((workspace: Workspace) => dispatch({ type: "UPDATE_WORKSPACE", workspace }), []),
  };
}

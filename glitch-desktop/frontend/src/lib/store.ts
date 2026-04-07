import { useReducer, useCallback } from "react";
import type {
  Message,
  Block,
  ChainStepChip,
  SystemStatus,
  Workspace,
} from "./types";

interface ChatState {
  messages: Message[];
  streaming: boolean;
  /** Most recent transient status from the agent, e.g. "scanning workflows".
   * Cleared when streaming finishes. Drives the "gl1tch is thinking" pill. */
  thinking: string;
  status: SystemStatus;
  sidebarOpen: boolean;
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
}

/**
 * Wire-format block event from the backend protocol splitter. Matches the
 * map produced by encodeBlockEvent in glitch-desktop/app.go.
 */
export interface WireBlockEvent {
  kind: "start" | "chunk" | "end";
  block: string;            // "text" | "note" | "table" | "code" | "status"
  attrs?: Record<string, string>;
  text?: string;
}

type Action =
  | { type: "ADD_USER_MESSAGE"; text: string }
  | { type: "ADD_USER_CHAIN"; steps: ChainStepChip[]; text?: string }
  | { type: "APPLY_BLOCK_EVENT"; event: WireBlockEvent }
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

/**
 * Reducer for incoming protocol BlockEvents.
 *
 * The backend splitter sends START → CHUNK* → END for every block. Our job
 * is to materialize that into the in-memory `Block[]` of the most recent
 * (assistant) message:
 *
 *   - "text"   → append to / extend the trailing text Block
 *   - "note"   → buffered into a Block of type "brain" with markdown body
 *   - "table"  → emitted as a `text` Block (markdown handles the table)
 *   - "code"   → emitted as a `code` Block with lang/file from attrs
 *   - "status" → not added to the message body; lifts to state.thinking
 *
 * Unknown block types fall through to text so we never lose data.
 */
function applyBlockEvent(state: ChatState, ev: WireBlockEvent): ChatState {
  if (state.messages.length === 0) return state;

  // STATUS pings update the transient thinking pill and never enter the
  // message body — the agent can replace them as it works.
  if (ev.block === "status") {
    if (ev.kind === "chunk" && ev.text) {
      return { ...state, thinking: ev.text.trim() || state.thinking };
    }
    if (ev.kind === "end") {
      // Leave the most recent text in place until the next status arrives;
      // a final FINISH_ASSISTANT will clear it.
      return state;
    }
    return state;
  }

  const msgs = [...state.messages];
  const lastIdx = msgs.length - 1;
  const last = { ...msgs[lastIdx] };
  let blocks = [...last.blocks];

  if (ev.kind === "start") {
    blocks = openBlock(blocks, ev);
    // First non-status content arrived — drop the "thinking…" placeholder.
    last.blocks = blocks;
    msgs[lastIdx] = last;
    return { ...state, thinking: "", messages: msgs };
  }

  if (ev.kind === "chunk") {
    blocks = appendToOpenBlock(blocks, ev);
    last.blocks = blocks;
    msgs[lastIdx] = last;
    return { ...state, thinking: "", messages: msgs };
  }

  // end: nothing to mutate — the open block stays in the array. We use
  // start/end markers only for the splitter's internal state on the wire;
  // the frontend treats blocks as immutable once chunks have appended.
  return state;
}

function openBlock(blocks: Block[], ev: WireBlockEvent): Block[] {
  const attrs = ev.attrs ?? {};
  switch (ev.block) {
    case "note": {
      const tags = (attrs.tags ?? "").split(",").map((t) => t.trim()).filter(Boolean);
      return [
        ...blocks,
        {
          type: "brain",
          note: { brainType: attrs.type, title: attrs.title, tags, content: "" },
        },
      ];
    }
    case "code":
      return [
        ...blocks,
        { type: "code", language: attrs.lang ?? "", filename: attrs.file, content: "" },
      ];
    case "table":
      // Tables ride inside a normal text block — react-markdown + remark-gfm
      // already render pipe tables, so we just collect the markdown body.
      return [...blocks, { type: "text", content: "" }];
    case "text":
    default:
      return [...blocks, { type: "text", content: "" }];
  }
}

function appendToOpenBlock(blocks: Block[], ev: WireBlockEvent): Block[] {
  if (blocks.length === 0) return blocks;
  const idx = blocks.length - 1;
  const tail = blocks[idx];
  const text = ev.text ?? "";

  if (tail.type === "text") {
    return [...blocks.slice(0, idx), { ...tail, content: tail.content + text }];
  }
  if (tail.type === "code") {
    return [...blocks.slice(0, idx), { ...tail, content: tail.content + text }];
  }
  if (tail.type === "brain") {
    return [
      ...blocks.slice(0, idx),
      { ...tail, note: { ...tail.note, content: tail.note.content + text } },
    ];
  }
  // Unknown tail type — append a fresh text block to avoid losing data.
  return [...blocks, { type: "text", content: text }];
}

const initialState: ChatState = {
  messages: [],
  streaming: false,
  thinking: "",
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

    case "ADD_USER_CHAIN": {
      const blocks: Block[] = [
        { type: "chain", steps: action.steps, text: action.text },
      ];
      return {
        ...state,
        messages: [
          ...state.messages,
          {
            id: makeId(),
            role: "user",
            blocks,
            timestamp: Date.now(),
          },
        ],
      };
    }

    case "START_ASSISTANT":
      return {
        ...state,
        streaming: true,
        // Default thinking message before the agent emits its own status.
        // The chat shows this immediately so the user knows we're waiting.
        thinking: "thinking…",
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
      return { ...state, streaming: false, thinking: "", messages: msgs };
    }

    case "STREAM_ERROR": {
      const msgs = [...state.messages];
      const last = { ...msgs[msgs.length - 1] };
      last.streaming = false;
      last.blocks = [...last.blocks, { type: "error", message: action.message }];
      msgs[msgs.length - 1] = last;
      return { ...state, streaming: false, thinking: "", messages: msgs };
    }

    case "APPLY_BLOCK_EVENT":
      return applyBlockEvent(state, action.event);

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
    applyBlockEvent: useCallback(
      (event: WireBlockEvent) => dispatch({ type: "APPLY_BLOCK_EVENT", event }),
      [],
    ),
    addUserChain: useCallback(
      (steps: ChainStepChip[], text?: string) => dispatch({ type: "ADD_USER_CHAIN", steps, text }),
      [],
    ),
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

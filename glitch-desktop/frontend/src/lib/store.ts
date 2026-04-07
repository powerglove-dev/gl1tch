import { useReducer, useCallback } from "react";
import type {
  Message,
  Block,
  ChainStepChip,
  SystemStatus,
  Workspace,
  BrainActivity,
} from "./types";

/**
 * Per-workspace chat slice. Each workspace has its own message buffer,
 * streaming flag, and "thinking" status text. The active view is just one
 * of these slices keyed by activeWorkspaceId. Keeping this state per
 * workspace is what lets the user switch away from a workspace whose
 * agent is still working without (a) blocking the input on the new
 * workspace, (b) corrupting the new workspace's history with chunks meant
 * for the old one, or (c) losing the in-flight assistant message of the
 * workspace they just left.
 */
interface WorkspaceChat {
  messages: Message[];
  streaming: boolean;
  /** Most recent transient status from the agent, e.g. "scanning workflows".
   * Cleared when streaming finishes. Drives the "gl1tch is thinking" pill. */
  thinking: string;
  /** True once we've hydrated this workspace from the DB at least once.
   *  Prevents re-fetching on every workspace switch. */
  loaded: boolean;
}

interface ChatState {
  byWorkspace: Record<string, WorkspaceChat>;
  status: SystemStatus;
  sidebarOpen: boolean;
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
  /** Rolling buffer of brain activity entries (alerts + check-ins),
   *  newest first. Capped to keep memory bounded. */
  brainActivity: BrainActivity[];
}

const BRAIN_ACTIVITY_CAP = 200;

/**
 * Wire-format block event from the backend protocol splitter. Matches the
 * map produced by encodeBlockEvent in glitch-desktop/app.go.
 */
export interface WireBlockEvent {
  workspace_id?: string;
  kind: "start" | "chunk" | "end";
  block: string;            // "text" | "note" | "table" | "code" | "status"
  attrs?: Record<string, string>;
  text?: string;
}

type Action =
  | { type: "ADD_USER_MESSAGE"; workspaceId: string; text: string }
  | { type: "ADD_USER_CHAIN"; workspaceId: string; steps: ChainStepChip[]; text?: string }
  | { type: "APPLY_BLOCK_EVENT"; workspaceId: string; event: WireBlockEvent }
  | { type: "START_ASSISTANT"; workspaceId: string }
  | { type: "APPEND_CHUNK"; workspaceId: string; text: string }
  | { type: "APPEND_BLOCK"; workspaceId: string; block: Block }
  | { type: "FINISH_ASSISTANT"; workspaceId: string; meta?: { model?: string; tokens?: number; latency_ms?: number } }
  | { type: "STREAM_ERROR"; workspaceId: string; message: string }
  | { type: "SET_STATUS"; status: Partial<SystemStatus> }
  | { type: "TOGGLE_SIDEBAR" }
  | { type: "CLEAR_MESSAGES"; workspaceId: string }
  | { type: "SET_MESSAGES"; workspaceId: string; messages: Message[] }
  | { type: "SET_WORKSPACES"; workspaces: Workspace[] }
  | { type: "SET_ACTIVE_WORKSPACE"; id: string | null }
  | { type: "ADD_WORKSPACE"; workspace: Workspace }
  | { type: "REMOVE_WORKSPACE"; id: string }
  | { type: "UPDATE_WORKSPACE"; workspace: Workspace }
  | { type: "ADD_BRAIN_ACTIVITY"; entry: BrainActivity }
  | { type: "MARK_BRAIN_READ" }
  | { type: "CLEAR_BRAIN_ACTIVITY" };

let nextId = 0;
function makeId() {
  return `msg-${Date.now()}-${nextId++}`;
}

function emptySlice(): WorkspaceChat {
  return { messages: [], streaming: false, thinking: "", loaded: false };
}

/**
 * Lift a per-workspace mutator into a state update. Ensures the slice
 * exists (creating an empty one if not), runs `fn`, and writes it back.
 * Returns the original state if `fn` returns the same object reference,
 * which keeps the reducer's identity-equality semantics for React.
 */
function withSlice(
  state: ChatState,
  workspaceId: string,
  fn: (slice: WorkspaceChat) => WorkspaceChat,
): ChatState {
  const current = state.byWorkspace[workspaceId] ?? emptySlice();
  const next = fn(current);
  if (next === current) return state;
  return {
    ...state,
    byWorkspace: { ...state.byWorkspace, [workspaceId]: next },
  };
}

/**
 * Reducer for incoming protocol BlockEvents.
 *
 * The backend splitter sends START → CHUNK* → END for every block. Our job
 * is to materialize that into the in-memory `Block[]` of the most recent
 * (assistant) message in the *target workspace's* slice:
 *
 *   - "text"   → append to / extend the trailing text Block
 *   - "note"   → buffered into a Block of type "brain" with markdown body
 *   - "table"  → emitted as a `text` Block (markdown handles the table)
 *   - "code"   → emitted as a `code` Block with lang/file from attrs
 *   - "status" → not added to the message body; lifts to slice.thinking
 *
 * Unknown block types fall through to text so we never lose data.
 */
function applyBlockEvent(slice: WorkspaceChat, ev: WireBlockEvent): WorkspaceChat {
  if (slice.messages.length === 0) return slice;

  // STATUS pings update the transient thinking pill and never enter the
  // message body — the agent can replace them as it works.
  if (ev.block === "status") {
    if (ev.kind === "chunk" && ev.text) {
      return { ...slice, thinking: ev.text.trim() || slice.thinking };
    }
    return slice;
  }

  const msgs = [...slice.messages];
  const lastIdx = msgs.length - 1;
  const last = { ...msgs[lastIdx] };
  let blocks = [...last.blocks];

  if (ev.kind === "start") {
    blocks = openBlock(blocks, ev);
    last.blocks = blocks;
    msgs[lastIdx] = last;
    return { ...slice, thinking: "", messages: msgs };
  }

  if (ev.kind === "chunk") {
    blocks = appendToOpenBlock(blocks, ev);
    last.blocks = blocks;
    msgs[lastIdx] = last;
    return { ...slice, thinking: "", messages: msgs };
  }

  // end: nothing to mutate — the open block stays in the array. We use
  // start/end markers only for the splitter's internal state on the wire;
  // the frontend treats blocks as immutable once chunks have appended.
  return slice;
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
  byWorkspace: {},
  status: { ollama: false, elasticsearch: false, busd: false, brain: "idle", brainDetail: "" },
  sidebarOpen: true,
  workspaces: [],
  activeWorkspaceId: null,
  brainActivity: [],
};

function reducer(state: ChatState, action: Action): ChatState {
  switch (action.type) {
    case "ADD_USER_MESSAGE":
      return withSlice(state, action.workspaceId, (s) => ({
        ...s,
        messages: [
          ...s.messages,
          {
            id: makeId(),
            role: "user",
            blocks: [{ type: "text", content: action.text }],
            timestamp: Date.now(),
          },
        ],
      }));

    case "ADD_USER_CHAIN":
      return withSlice(state, action.workspaceId, (s) => ({
        ...s,
        messages: [
          ...s.messages,
          {
            id: makeId(),
            role: "user",
            blocks: [{ type: "chain", steps: action.steps, text: action.text }],
            timestamp: Date.now(),
          },
        ],
      }));

    case "START_ASSISTANT":
      return withSlice(state, action.workspaceId, (s) => ({
        ...s,
        streaming: true,
        // Default thinking message before the agent emits its own status.
        thinking: "thinking…",
        messages: [
          ...s.messages,
          {
            id: makeId(),
            role: "assistant",
            blocks: [],
            timestamp: Date.now(),
            streaming: true,
          },
        ],
      }));

    case "APPEND_CHUNK":
      return withSlice(state, action.workspaceId, (s) => {
        if (s.messages.length === 0) return s;
        const msgs = [...s.messages];
        const last = { ...msgs[msgs.length - 1] };
        const blocks = [...last.blocks];
        const tail = blocks[blocks.length - 1];
        if (tail && tail.type === "text") {
          blocks[blocks.length - 1] = { ...tail, content: tail.content + action.text };
        } else {
          blocks.push({ type: "text", content: action.text });
        }
        last.blocks = blocks;
        msgs[msgs.length - 1] = last;
        return { ...s, messages: msgs };
      });

    case "APPEND_BLOCK":
      return withSlice(state, action.workspaceId, (s) => {
        if (s.messages.length === 0) return s;
        const msgs = [...s.messages];
        const last = { ...msgs[msgs.length - 1] };
        last.blocks = [...last.blocks, action.block];
        msgs[msgs.length - 1] = last;
        return { ...s, messages: msgs };
      });

    case "FINISH_ASSISTANT":
      return withSlice(state, action.workspaceId, (s) => {
        if (s.messages.length === 0) return { ...s, streaming: false, thinking: "" };
        const msgs = [...s.messages];
        const last = { ...msgs[msgs.length - 1] };
        last.streaming = false;
        if (action.meta) {
          last.blocks = [...last.blocks, { type: "done" as const, ...action.meta }];
        }
        msgs[msgs.length - 1] = last;
        return { ...s, streaming: false, thinking: "", messages: msgs };
      });

    case "STREAM_ERROR":
      return withSlice(state, action.workspaceId, (s) => {
        if (s.messages.length === 0) return { ...s, streaming: false, thinking: "" };
        const msgs = [...s.messages];
        const last = { ...msgs[msgs.length - 1] };
        last.streaming = false;
        last.blocks = [...last.blocks, { type: "error", message: action.message }];
        msgs[msgs.length - 1] = last;
        return { ...s, streaming: false, thinking: "", messages: msgs };
      });

    case "APPLY_BLOCK_EVENT":
      return withSlice(state, action.workspaceId, (s) => applyBlockEvent(s, action.event));

    case "SET_STATUS":
      return { ...state, status: { ...state.status, ...action.status } };

    case "TOGGLE_SIDEBAR":
      return { ...state, sidebarOpen: !state.sidebarOpen };

    case "CLEAR_MESSAGES":
      return withSlice(state, action.workspaceId, (s) =>
        s.messages.length === 0 && !s.thinking && !s.streaming ? s : { ...s, messages: [], thinking: "", streaming: false },
      );

    case "SET_MESSAGES":
      return withSlice(state, action.workspaceId, (s) => ({
        ...s,
        messages: action.messages,
        loaded: true,
      }));

    case "SET_WORKSPACES":
      return { ...state, workspaces: action.workspaces };

    case "SET_ACTIVE_WORKSPACE":
      return { ...state, activeWorkspaceId: action.id };

    case "ADD_WORKSPACE":
      return { ...state, workspaces: [action.workspace, ...state.workspaces] };

    case "REMOVE_WORKSPACE": {
      const { [action.id]: _removed, ...remaining } = state.byWorkspace;
      void _removed;
      return {
        ...state,
        workspaces: state.workspaces.filter((w) => w.id !== action.id),
        activeWorkspaceId: state.activeWorkspaceId === action.id ? null : state.activeWorkspaceId,
        byWorkspace: remaining,
      };
    }

    case "UPDATE_WORKSPACE":
      return {
        ...state,
        workspaces: state.workspaces.map((w) =>
          w.id === action.workspace.id ? action.workspace : w,
        ),
      };

    case "ADD_BRAIN_ACTIVITY": {
      // Newest first; cap to BRAIN_ACTIVITY_CAP. If we already have an entry
      // with the same id, treat it as an update (drop the dup so the newer
      // copy moves to the top).
      const filtered = state.brainActivity.filter((e) => e.id !== action.entry.id);
      const next = [action.entry, ...filtered].slice(0, BRAIN_ACTIVITY_CAP);
      // If a new alert lands and the brain isn't currently in error/analyzing,
      // promote the brain state to "alert" so the indicator demands attention.
      const promote =
        action.entry.kind === "alert" &&
        action.entry.severity !== "info" &&
        state.status.brain !== "error";
      return {
        ...state,
        brainActivity: next,
        status: promote ? { ...state.status, brain: "alert" } : state.status,
      };
    }

    case "MARK_BRAIN_READ":
      return {
        ...state,
        brainActivity: state.brainActivity.map((e) => (e.unread ? { ...e, unread: false } : e)),
      };

    case "CLEAR_BRAIN_ACTIVITY":
      return { ...state, brainActivity: [] };

    default:
      return state;
  }
}

const EMPTY_SLICE: WorkspaceChat = emptySlice();

export function useChatStore() {
  const [state, dispatch] = useReducer(reducer, initialState);

  // Active slice — what the chat surface is currently showing. Falls
  // back to the empty slice when no workspace is selected so consumers
  // can read messages/streaming/thinking unconditionally.
  const active = state.activeWorkspaceId
    ? state.byWorkspace[state.activeWorkspaceId] ?? EMPTY_SLICE
    : EMPTY_SLICE;

  return {
    state,
    active,
    addUserMessage: useCallback(
      (workspaceId: string, text: string) => dispatch({ type: "ADD_USER_MESSAGE", workspaceId, text }),
      [],
    ),
    addUserChain: useCallback(
      (workspaceId: string, steps: ChainStepChip[], text?: string) =>
        dispatch({ type: "ADD_USER_CHAIN", workspaceId, steps, text }),
      [],
    ),
    applyBlockEvent: useCallback(
      (workspaceId: string, event: WireBlockEvent) =>
        dispatch({ type: "APPLY_BLOCK_EVENT", workspaceId, event }),
      [],
    ),
    startAssistant: useCallback(
      (workspaceId: string) => dispatch({ type: "START_ASSISTANT", workspaceId }),
      [],
    ),
    appendChunk: useCallback(
      (workspaceId: string, text: string) => dispatch({ type: "APPEND_CHUNK", workspaceId, text }),
      [],
    ),
    appendBlock: useCallback(
      (workspaceId: string, block: Block) => dispatch({ type: "APPEND_BLOCK", workspaceId, block }),
      [],
    ),
    finishAssistant: useCallback(
      (workspaceId: string, meta?: { model?: string; tokens?: number; latency_ms?: number }) =>
        dispatch({ type: "FINISH_ASSISTANT", workspaceId, meta }),
      [],
    ),
    streamError: useCallback(
      (workspaceId: string, message: string) => dispatch({ type: "STREAM_ERROR", workspaceId, message }),
      [],
    ),
    setStatus: useCallback((status: Partial<SystemStatus>) => dispatch({ type: "SET_STATUS", status }), []),
    toggleSidebar: useCallback(() => dispatch({ type: "TOGGLE_SIDEBAR" }), []),
    clearMessages: useCallback(
      (workspaceId: string) => dispatch({ type: "CLEAR_MESSAGES", workspaceId }),
      [],
    ),
    setMessages: useCallback(
      (workspaceId: string, messages: Message[]) =>
        dispatch({ type: "SET_MESSAGES", workspaceId, messages }),
      [],
    ),
    setWorkspaces: useCallback(
      (workspaces: Workspace[]) => dispatch({ type: "SET_WORKSPACES", workspaces }),
      [],
    ),
    setActiveWorkspace: useCallback(
      (id: string | null) => dispatch({ type: "SET_ACTIVE_WORKSPACE", id }),
      [],
    ),
    addWorkspace: useCallback(
      (workspace: Workspace) => dispatch({ type: "ADD_WORKSPACE", workspace }),
      [],
    ),
    removeWorkspace: useCallback((id: string) => dispatch({ type: "REMOVE_WORKSPACE", id }), []),
    updateWorkspace: useCallback(
      (workspace: Workspace) => dispatch({ type: "UPDATE_WORKSPACE", workspace }),
      [],
    ),
    addBrainActivity: useCallback(
      (entry: BrainActivity) => dispatch({ type: "ADD_BRAIN_ACTIVITY", entry }),
      [],
    ),
    markBrainRead: useCallback(() => dispatch({ type: "MARK_BRAIN_READ" }), []),
    clearBrainActivity: useCallback(() => dispatch({ type: "CLEAR_BRAIN_ACTIVITY" }), []),
  };
}

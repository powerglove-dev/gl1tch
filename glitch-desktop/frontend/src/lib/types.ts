/** Message roles */
export type Role = "user" | "assistant";

/** One step rendered as a chip in a ChainBlock. */
export interface ChainStepChip {
  label: string;
  kind: "prompt" | "agent" | "pipeline";
  provider?: string;
  model?: string;
}

/** A tool call extracted from CLI agent output (copilot, claude, …). */
export interface ToolCall {
  ok: boolean;
  label: string;
  tool: string;
  command?: string;
  result?: string;
}

/** A `<brain type="…" tags="…" title="…">…</brain>` note pulled out of agent output. */
export interface BrainNote {
  brainType?: string;
  title?: string;
  tags: string[];
  /** Markdown content of the note. */
  content: string;
}

/** A single block within a message */
export type Block =
  | { type: "text"; content: string }
  | { type: "code"; language: string; filename?: string; content: string }
  | {
      type: "chain";
      steps: ChainStepChip[];
      /** Optional free-text the user typed after the chain chips. */
      text?: string;
    }
  | {
      type: "activity";
      /** Summary header lines pulled from CLI agent output (e.g. token counts). */
      summary: string[];
      tools: ToolCall[];
    }
  | {
      type: "brain";
      note: BrainNote;
    }
  | {
      type: "table";
      headers: string[];
      rows: string[][];
    }
  | {
      type: "action";
      id: string;
      label: string;
      method: string;
      args?: unknown[];
    }
  | { type: "status"; text: string }
  | { type: "link"; url: string; title: string; description?: string }
  | { type: "error"; message: string }
  | {
      type: "done";
      model?: string;
      tokens?: number;
      latency_ms?: number;
    };

/** A chat message (one or more blocks) */
export interface Message {
  id: string;
  role: Role;
  blocks: Block[];
  timestamp: number;
  streaming?: boolean;
  /**
   * Optional daemon-origin metadata attached when the backend
   * proactively injected this message (e.g. the attention classifier
   * dropped a high-attention artifact into chat without the user
   * typing first). Used by the activity sidebar's "↗ in chat" jump
   * affordance to scroll to the right message by event_key.
   */
  injected?: {
    event_key?: string;
    source?: string;
    repo?: string;
    attention?: "high" | "normal" | "low";
    reason?: string;
    title?: string;
  };
}

/** Workspace — a chat session with its own directories */
export interface Workspace {
  id: string;
  title: string;
  directories: string[] | null;
  repo_names: string[] | null;
  primary_directory?: string;
  created_at: number;
  updated_at: number;
}

/** Pipeline definition */
export interface PipelineDef {
  name: string;
  description: string;
  path: string;
  steps: number;
}

/** Pipeline run status */
export interface PipelineRun {
  id: string;
  pipeline: string;
  status: "running" | "done" | "error";
  output: string;
  startedAt: number;
}

/** Agent status */
export interface AgentInfo {
  id: string;
  name: string;
  provider: string;
  model: string;
  status: "running" | "stopped" | "error";
}

/** Activity event from observer */
export interface ActivityEvent {
  id: string;
  kind: string;
  title: string;
  detail: string;
  timestamp: number;
}

/** System status */
export interface SystemStatus {
  ollama: boolean;
  elasticsearch: boolean;
  busd: boolean;
  /** Live brain state — drives the persistent brain icon. */
  brain: BrainState;
  /** Short human-readable detail for the icon tooltip. */
  brainDetail: string;
}

/** Visual states the persistent brain indicator can be in. */
export type BrainState =
  | "idle"        // nothing happening
  | "collecting"  // background collectors running, brain is "watching"
  | "analyzing"   // local model is triaging buffered items right now
  | "alert"       // unread alert(s) waiting for the user
  | "error";      // local model unreachable / brain offline

/** Severity for a brain alert / activity entry. */
export type BrainSeverity = "info" | "warn" | "error";

/**
 * One preview item attached to an indexing-kind activity row. The
 * backend's collector refresh query fetches the top 5 most recent
 * newly-indexed docs per source and serializes them on the event
 * payload so the sidebar can render "N new + top 5 titles" inline
 * without a separate fetch. The drill-in modal re-queries for the
 * full set when the user clicks "View all".
 */
export interface ActivityItem {
  source: string;
  type?: string;
  repo?: string;
  author?: string;
  sha?: string;
  title: string;
  url?: string;
  timestamp_ms?: number;
}

/**
 * One indexed document in the drill-in modal's list pane. Shape
 * matches pkg/glitchd RecentEvent (serialized through the
 * ListIndexedDocs Wails binding). Files is optional because some
 * sources don't track file-level changes.
 */
export interface IndexedDoc {
  type?: string;
  source: string;
  repo?: string;
  branch?: string;
  author?: string;
  sha?: string;
  message?: string;
  body?: string;
  files?: string[];
  url?: string;
  timestamp_ms?: number;
}

/**
 * Stream event delivered over the brain:analysis:stream Wails event
 * from AnalyzeActivityChunks. Tokens arrive as kind="token" with
 * the delta text; the stream terminates with kind="done" (success)
 * or kind="error" (failure). Error carries the human-readable
 * reason.
 */
export interface AnalysisStreamEvent {
  streamId: string;
  kind: "token" | "done" | "error";
  data?: string;
  error?: string;
}

/**
 * One entry in the Activity panel. The brain emits three flavors:
 *  - "alert": something the user should look at (severity warn/error)
 *  - "checkin": low-noise periodic status ("watching", "stored 12 commits…")
 *  - "analysis": a deep-analysis run from the opencode-driven analyzer.
 *                detail is full markdown the renderer expands inline.
 */
export interface BrainActivity {
  id: string;
  /** "alert" surfaces in the systray; "checkin" + "analysis" stay in-app only. */
  kind: "alert" | "checkin" | "analysis";
  severity: BrainSeverity;
  title: string;
  /** One-line reason / summary, OR — for kind="analysis" — full markdown body. */
  detail: string;
  /** Optional source pointer (collector name, file path, run id, …). */
  source?: string;
  /** kind="analysis" extras populated by the deep-analysis loop. */
  repo?: string;
  event_type?: string;
  event_key?: string;
  model?: string;
  duration_ms?: number;
  workspace_id?: string;
  timestamp: number;
  /** True until the user has opened the brain panel after this landed. */
  unread: boolean;
  /** Indexing-kind extras: preview items + delta bookkeeping so the
   *  row can render inline previews and the drill-in modal can open
   *  scoped to the right time window. All optional — older-shape
   *  activity events without these fields still render. */
  items?: ActivityItem[];
  delta?: number;
  source_total?: number;
  last_seen_ms?: number;
  window_from_ms?: number;
  /** Analysis refinement chain pointer. When present, the analysis
   *  row refines an earlier analysis with this event_key. Used by
   *  the frontend to render threaded chains; purely informational. */
  parent_id?: string;
  /** Attention classifier verdict for the underlying event.
   *  Populated on kind="analysis" rows when the deep analyzer ran
   *  and on kind="alert" rows emitted by the attention observer
   *  nudge path ("flagged high but analysis disabled"). Drives the
   *  small dot badge in the activity row. */
  attention?: "high" | "normal" | "low";
  attention_reason?: string;
  /** True when this analysis row's full artifact was already
   *  injected into the chat pane as an assistant message (the
   *  proactive high-attention path). The row's header gets an
   *  "↗ in chat" jump affordance that scrolls the chat to the
   *  matching message by event_key. */
  chat_injected?: boolean;
  /** True on the "flagged high · analysis disabled" nudge row so
   *  the frontend can render a distinct CTA inviting the user to
   *  enable deep analysis in settings. */
  needs_enable?: boolean;
  /** Optional title of the underlying event the nudge refers to
   *  (separate from the row's own title, which is generic). */
  title_event?: string;
}

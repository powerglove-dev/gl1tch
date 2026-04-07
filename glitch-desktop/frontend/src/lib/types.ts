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
}

/** Workspace — a chat session with its own directories */
export interface Workspace {
  id: string;
  title: string;
  directories: string[] | null;
  repo_names: string[] | null;
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
  brain: "idle" | "improving";
  brainDetail: string;
}

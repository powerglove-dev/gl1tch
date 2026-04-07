/**
 * collectorSchema — single source of truth for the structured config
 * modal. Each collector declares its fields once here and the modal
 * renders them via FieldInput; adding a new collector is "append a
 * CollectorSpec" with no other UI work.
 *
 * The shape mirrors internal/collector/config.go's Config struct so
 * the JSON round-trip via GetCollectorsConfigJSON / WriteCollectorsConfigJSON
 * stays direct — each FieldSpec.key is the dotted path into Config.
 *
 * If you add a field to internal/collector/config.go, add a matching
 * FieldSpec entry here. The two files are intentionally coupled: the
 * Go struct is the contract, this file is the UI presentation of it.
 */

export type FieldType =
  | "boolean"
  | "string"
  | "secret"
  | "number"
  | "duration"
  | "string-list"
  | "path-list"
  | "enum";

export interface FieldSpec {
  /** Dotted path into the Config struct, e.g. "code_index.paths". */
  key: string;
  label: string;
  description?: string;
  type: FieldType;
  required?: boolean;
  /** Used for enum fields. */
  enumValues?: string[];
  /** Numeric bounds (also used by duration via parsed seconds). */
  min?: number;
  max?: number;
  /** Hide this field unless the predicate over current form values is true. */
  visibleWhen?: (values: Record<string, unknown>) => boolean;
  /** Placeholder shown when the field is empty. */
  placeholder?: string;
  /** When true the field renders disabled. Used for fields whose
   *  value is auto-derived elsewhere (e.g. git.repos comes from
   *  workspace directories via AutoDetectFromWorkspace, so editing
   *  it in the modal would be a no-op on the next read). */
  readOnly?: boolean;
}

export interface CollectorSpec {
  /** Stable id used for routing in the modal sidebar. Matches the
   *  collector "name" returned by ListCollectors so the row → modal
   *  jump can pre-select the right entry. */
  id: string;
  name: string;
  /** lucide-react icon name; resolved at render time so this file
   *  stays free of React imports. */
  icon: string;
  description: string;
  /** Dotted path of the master enabled flag. The modal renders this
   *  as a single switch in the header so the user can flip the whole
   *  collector off without clearing every field. */
  enabledKey?: string;
  /** "true if the collector is effectively enabled" — some collectors
   *  (git, github, mattermost) auto-enable when their list/credentials
   *  are populated rather than via an explicit boolean. The schema
   *  encodes that so the sidebar can show the right green dot. */
  isEnabled: (values: Record<string, unknown>) => boolean;
  fields: FieldSpec[];
}

// ── Field path helpers ─────────────────────────────────────────────────
//
// The Config object we get from GetCollectorsConfigJSON is nested
// (cfg.code_index.paths, cfg.git.repos, …). The modal flattens reads
// and writes through these helpers so each FieldInput only deals with
// a single key string.

export function getField(values: Record<string, unknown>, key: string): unknown {
  const parts = key.split(".");
  let cur: unknown = values;
  for (const p of parts) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = (cur as Record<string, unknown>)[p];
  }
  return cur;
}

export function setField(
  values: Record<string, unknown>,
  key: string,
  value: unknown,
): Record<string, unknown> {
  const parts = key.split(".");
  // Shallow clone every level on the path so React state updates
  // remain immutable.
  const root: Record<string, unknown> = { ...values };
  let cur: Record<string, unknown> = root;
  for (let i = 0; i < parts.length - 1; i++) {
    const p = parts[i];
    const next = cur[p];
    cur[p] = next && typeof next === "object" ? { ...(next as object) } : {};
    cur = cur[p] as Record<string, unknown>;
  }
  cur[parts[parts.length - 1]] = value;
  return root;
}

// ── Duration helpers ───────────────────────────────────────────────────
//
// Go's time.Duration marshals to a number of nanoseconds in JSON.
// The duration field type accepts strings like "30m" / "60s" from
// the user and converts in both directions.

const NS_PER_MS = 1_000_000;
const NS_PER_S = 1000 * NS_PER_MS;
const NS_PER_M = 60 * NS_PER_S;
const NS_PER_H = 60 * NS_PER_M;

export function parseDuration(s: string): number | null {
  const m = s.trim().match(/^(\d+)\s*(ms|s|m|h)?$/i);
  if (!m) return null;
  const n = parseInt(m[1], 10);
  if (!Number.isFinite(n)) return null;
  const unit = (m[2] || "s").toLowerCase();
  switch (unit) {
    case "ms":
      return n * NS_PER_MS;
    case "s":
      return n * NS_PER_S;
    case "m":
      return n * NS_PER_M;
    case "h":
      return n * NS_PER_H;
  }
  return null;
}

export function formatDuration(ns: number | null | undefined): string {
  if (ns == null || !Number.isFinite(ns) || ns <= 0) return "";
  if (ns % NS_PER_H === 0) return `${ns / NS_PER_H}h`;
  if (ns % NS_PER_M === 0) return `${ns / NS_PER_M}m`;
  if (ns % NS_PER_S === 0) return `${ns / NS_PER_S}s`;
  if (ns % NS_PER_MS === 0) return `${ns / NS_PER_MS}ms`;
  return `${ns}ns`;
}

// ── The schema ─────────────────────────────────────────────────────────

export const COLLECTOR_SCHEMA: CollectorSpec[] = [
  {
    id: "git",
    name: "Git",
    icon: "GitBranch",
    description:
      "Watches local repositories and indexes new commits as they land.",
    isEnabled: (v) => {
      const repos = getField(v, "git.repos");
      return Array.isArray(repos) && repos.length > 0;
    },
    fields: [
      {
        key: "git.repos",
        label: "Repositories",
        description:
          "Auto-detected from your workspace directories — any directory containing a .git checkout is added here. To change the list, edit the Directories collector instead.",
        type: "path-list",
        readOnly: true,
      },
      {
        key: "git.interval",
        label: "Poll interval",
        description: "How often to check for new commits.",
        type: "duration",
        placeholder: "60s",
      },
    ],
  },
  {
    id: "claude",
    name: "Claude Code",
    icon: "Bot",
    description:
      "Indexes Claude Code conversation history (~/.claude/) so the brain can recall what you've been doing across sessions.",
    enabledKey: "claude.enabled",
    isEnabled: (v) => !!getField(v, "claude.enabled"),
    fields: [
      {
        key: "claude.interval",
        label: "Poll interval",
        type: "duration",
        placeholder: "120s",
      },
    ],
  },
  {
    id: "copilot",
    name: "GitHub Copilot",
    icon: "Bot",
    description:
      "Indexes GitHub Copilot CLI history and logs (~/.copilot/).",
    enabledKey: "copilot.enabled",
    isEnabled: (v) => !!getField(v, "copilot.enabled"),
    fields: [
      {
        key: "copilot.interval",
        label: "Poll interval",
        type: "duration",
        placeholder: "120s",
      },
    ],
  },
  {
    id: "github",
    name: "GitHub",
    icon: "Github",
    description:
      "Pulls PRs and issues from configured repos via the gh CLI (must be authenticated).",
    isEnabled: (v) => {
      const repos = getField(v, "github.repos");
      return Array.isArray(repos) && repos.length > 0;
    },
    fields: [
      {
        key: "github.repos",
        label: "Repositories",
        description:
          "Auto-detected from your workspace directories — any git checkout with a github.com origin is added here. To change the list, edit the Directories collector instead.",
        type: "string-list",
        readOnly: true,
      },
      {
        key: "github.interval",
        label: "Poll interval",
        type: "duration",
        placeholder: "5m",
      },
    ],
  },
  {
    id: "mattermost",
    name: "Mattermost",
    icon: "MessageSquare",
    description:
      "Joins channels and indexes messages so the brain can answer questions about team chat.",
    isEnabled: (v) =>
      !!getField(v, "mattermost.url") && !!getField(v, "mattermost.token"),
    fields: [
      {
        key: "mattermost.url",
        label: "Server URL",
        description: "Base URL of the Mattermost server.",
        type: "string",
        placeholder: "https://mattermost.example.com",
      },
      {
        key: "mattermost.token",
        label: "Bot token",
        description: "Personal access token or bot token.",
        type: "secret",
        placeholder: "$GLITCH_MATTERMOST_TOKEN or literal token",
      },
      {
        key: "mattermost.channels",
        label: "Channels",
        description: "Channel names to join. Empty = all visible channels.",
        type: "string-list",
        placeholder: "town-square",
      },
      {
        key: "mattermost.interval",
        label: "Poll interval",
        type: "duration",
        placeholder: "60s",
      },
    ],
  },
  {
    id: "directories",
    name: "Directories",
    icon: "Folder",
    description:
      "Scans project directories for agents, skills, and provider configs.",
    isEnabled: (v) => {
      const paths = getField(v, "directories.paths");
      return Array.isArray(paths) && paths.length > 0;
    },
    fields: [
      {
        key: "directories.paths",
        label: "Paths",
        description:
          "Directories you want gl1tch to watch. Git repos and GitHub remotes are auto-detected from these. Adds and removes here are synced into the workspace's directory list immediately on save.",
        type: "path-list",
      },
      {
        key: "directories.interval",
        label: "Re-scan interval",
        type: "duration",
        placeholder: "2m",
      },
    ],
  },
  {
    id: "code-index",
    name: "Code Index",
    icon: "Search",
    description:
      "Walks each path on an interval, chunks every source file, and stores semantic embeddings so the brain can answer 'where is this logic' questions. Disabled by default — first-run embedding can take several minutes for large trees.",
    enabledKey: "code_index.enabled",
    isEnabled: (v) =>
      !!getField(v, "code_index.enabled") &&
      Array.isArray(getField(v, "code_index.paths")) &&
      (getField(v, "code_index.paths") as unknown[]).length > 0,
    fields: [
      {
        key: "code_index.paths",
        label: "Paths to index",
        description:
          "Each root path becomes its own RAG scope so semantic search can target a single tree.",
        type: "path-list",
      },
      {
        key: "code_index.extensions",
        label: "File extensions",
        description: "Defaults to .go .ts .py .md when empty.",
        type: "string-list",
        placeholder: ".go",
      },
      {
        key: "code_index.chunk_size",
        label: "Chunk size (chars)",
        description: "Larger chunks → fewer embeddings, less precise recall.",
        type: "number",
        min: 100,
        max: 10000,
      },
      {
        key: "code_index.interval",
        label: "Re-index interval",
        description:
          "Re-walks all paths on this cadence. Unchanged files are nearly free thanks to hash-dedupe.",
        type: "duration",
        placeholder: "30m",
      },
      {
        key: "code_index.embed_provider",
        label: "Embedding provider",
        type: "enum",
        enumValues: ["ollama", "openai", "voyage"],
      },
      {
        key: "code_index.embed_model",
        label: "Embedding model",
        description:
          "Provider-specific. nomic-embed-text for small repos, mxbai-embed-large for >5k files.",
        type: "string",
        placeholder: "nomic-embed-text",
      },
      {
        key: "code_index.embed_base_url",
        label: "Ollama base URL",
        type: "string",
        placeholder: "http://localhost:11434",
        visibleWhen: (v) =>
          getField(v, "code_index.embed_provider") === "ollama" ||
          !getField(v, "code_index.embed_provider"),
      },
      {
        key: "code_index.embed_api_key",
        label: "API key",
        description: "Use $ENV_VAR to read from the environment.",
        type: "secret",
        placeholder: "$OPENAI_API_KEY",
        visibleWhen: (v) => {
          const p = getField(v, "code_index.embed_provider");
          return p === "openai" || p === "voyage";
        },
      },
    ],
  },
];

export function findCollectorById(id: string): CollectorSpec | undefined {
  return COLLECTOR_SCHEMA.find((c) => c.id === id);
}

/** Map a collector "name" returned by ListCollectors back to its
 *  schema id. ListCollectors uses display names like "git", "code-index";
 *  the schema uses the same ids so this is mostly identity, but we
 *  keep the lookup explicit so future renames are easy. */
export function schemaIdForCollectorName(name: string): string | undefined {
  const norm = name.toLowerCase().trim();
  const direct = COLLECTOR_SCHEMA.find((c) => c.id === norm);
  if (direct) return direct.id;
  // Tolerate underscores ↔ hyphens.
  const swapped = norm.replace(/_/g, "-");
  return COLLECTOR_SCHEMA.find((c) => c.id === swapped)?.id;
}

import type { ToolCall, BrainNote } from "./types";

/**
 * CLI agent plugins (copilot, claude-code, …) stream their native stdout
 * straight into the chat. That output bundles four things we want to show
 * differently:
 *
 *   1. Usage/summary preamble  ("Total usage est: …", "API time spent: …")
 *   2. Tool-call traces        ("× Search for hardcoded credentials (shell)\n  | grep …\n  └ Permission denied")
 *   3. Brain notes             ("<brain type='finding' tags='security' title='…'>…</brain>")
 *   4. The actual model response
 *
 * This parser splits a raw blob into structured pieces so the chat can render
 * each one with its own affordance instead of dumping everything into one
 * wall of text.
 */
export interface ParsedAgentOutput {
  summary: string[];
  tools: ToolCall[];
  brains: BrainNote[];
  body: string;
}

/**
 * gl1tch's own JSON stats sentinel emitted by glitchd at the end of each
 * step. Looks like:  `{"duration_ms":7425,"input_tokens":1038,"model":"llama3.2",
 * "output_tokens":289,"type":"gl1tch-stats"}`
 *
 * It's sometimes prefixed by `<>` (an arrow marker) or follows a
 * `[wrote: path]` GLITCH_WRITE acknowledgement on the same line.
 */
interface GlitchStats {
  duration_ms?: number;
  input_tokens?: number;
  output_tokens?: number;
  model?: string;
}

const STATS_RE = /\{[^{}]*"type"\s*:\s*"gl1tch-stats"[^{}]*\}/g;
const WROTE_RE = /\[wrote:\s*([^\]]+)\]/g;
const MARKER_PREFIX_RE = /^\s*<>\s*/;

function tryParseStats(blob: string): GlitchStats | null {
  try {
    return JSON.parse(blob);
  } catch {
    return null;
  }
}

function formatStats(s: GlitchStats): string[] {
  const out: string[] = [];
  if (s.model) out.push(`Model: ${s.model}`);
  if (s.duration_ms != null) out.push(`Duration: ${(s.duration_ms / 1000).toFixed(1)}s`);
  if (s.input_tokens != null || s.output_tokens != null) {
    const parts: string[] = [];
    if (s.input_tokens != null) parts.push(`${s.input_tokens.toLocaleString()} in`);
    if (s.output_tokens != null) parts.push(`${s.output_tokens.toLocaleString()} out`);
    out.push(`Tokens: ${parts.join(" · ")}`);
  }
  return out;
}

// Lines we treat as "run summary" noise — tokens, latency, model breakdown.
// Matched case-insensitively as a prefix of a trimmed line.
const SUMMARY_PREFIXES = [
  "total usage est",
  "api time spent",
  "total session time",
  "total code changes",
  "breakdown by ai model",
  // GitHub Copilot prints this as a continuation of the model breakdown.
  // Example: "claude-sonnet-4.6 93.9k in, 2.5k out, 38.8k cached (Est. 1 Premium request)"
  "claude-sonnet",
  "claude-haiku",
  "claude-opus",
  "gpt-",
  "gemini-",
];

function isSummaryLine(line: string): boolean {
  const lc = line.trim().toLowerCase();
  if (!lc) return false;
  return SUMMARY_PREFIXES.some((p) => lc.startsWith(p));
}

// A tool call header looks like one of:
//   ✓ Read file (read)
//   × Search for hardcoded credentials (shell)
//   ✗ List CI/CD workflow files (shell)
// followed (on subsequent lines) by `| <command>` and/or `└ <result>` rows
// drawn with box characters. The continuation rows may have leading whitespace.
const TOOL_HEADER_RE = /^([✓✔✗✘×])\s+(.+?)(?:\s+\(([A-Za-z0-9_-]+)\))?\s*$/;

function parseHeader(line: string): { ok: boolean; label: string; tool: string } | null {
  const m = line.match(TOOL_HEADER_RE);
  if (!m) return null;
  const [, mark, label, tool] = m;
  return {
    ok: mark === "✓" || mark === "✔",
    label: label.trim(),
    tool: (tool || "tool").trim(),
  };
}

// Continuation lines start (after stripping leading whitespace) with `|`/`│`
// for the command portion or `└` for the result portion.
function parseContinuation(line: string): { kind: "cmd" | "result"; text: string } | null {
  const s = line.replace(/^\s+/, "");
  if (s.startsWith("│") || s.startsWith("|")) {
    return { kind: "cmd", text: s.slice(1).trimStart() };
  }
  if (s.startsWith("└") || s.startsWith("┕")) {
    return { kind: "result", text: s.slice(1).trimStart() };
  }
  return null;
}

// Brain notes are emitted by gl1tch's prompt convention. They look like:
//   <brain type="finding" tags="security,ci-cd" title="Scan results">
//   freeform body, may span lines, may contain markdown
//   </brain>
// We extract them BEFORE running the line-based parser so attributes that
// happen to live on the same line as the closing tag don't confuse anything.
const BRAIN_RE = /<brain\b([^>]*)>([\s\S]*?)<\/brain>/gi;
const ATTR_RE = /(\w+)="([^"]*)"/g;

function parseAttrs(attrString: string): { brainType?: string; title?: string; tags: string[] } {
  const out: { brainType?: string; title?: string; tags: string[] } = { tags: [] };
  for (const m of attrString.matchAll(ATTR_RE)) {
    const key = m[1];
    const value = m[2];
    if (key === "type") out.brainType = value;
    else if (key === "title") out.title = value;
    else if (key === "tags") {
      out.tags = value.split(",").map((t) => t.trim()).filter(Boolean);
    }
  }
  return out;
}

/**
 * Pulls out gl1tch's own protocol noise so it doesn't pollute the chat body:
 *   - `{"...":"gl1tch-stats"}` JSON sentinels become structured summary lines
 *   - `[wrote: path]` GLITCH_WRITE acks become `wrote` tool entries
 *   - leading `<>` arrow markers are dropped
 */
function extractGlitchProtocol(raw: string): {
  stripped: string;
  summary: string[];
  tools: ToolCall[];
} {
  const summary: string[] = [];
  const tools: ToolCall[] = [];

  let stripped = raw.replace(STATS_RE, (match) => {
    const stats = tryParseStats(match);
    if (stats) summary.push(...formatStats(stats));
    return "";
  });

  stripped = stripped.replace(WROTE_RE, (_full, path: string) => {
    tools.push({
      ok: true,
      label: "Wrote file",
      tool: "write",
      command: path.trim(),
    });
    return "";
  });

  // Drop the now-orphaned `<>` arrow marker that prefixed the stats blob,
  // along with any blank line it leaves behind.
  stripped = stripped
    .split("\n")
    .map((line) => line.replace(MARKER_PREFIX_RE, ""))
    .join("\n");

  return { stripped, summary, tools };
}

function extractBrains(raw: string): { stripped: string; brains: BrainNote[] } {
  const brains: BrainNote[] = [];
  const stripped = raw.replace(BRAIN_RE, (_full, attrs: string, body: string) => {
    const attrParts = parseAttrs(attrs);
    brains.push({
      content: body.trim(),
      brainType: attrParts.brainType,
      title: attrParts.title,
      tags: attrParts.tags,
    });
    return ""; // remove from body; brains render in their own block
  });
  return { stripped, brains };
}

/**
 * Splits raw agent stdout into summary / tools / brains / body sections.
 *
 * This is a small line-driven state machine: while a tool header is "open",
 * any continuation lines (`|` command, `└` result, indented wraps) are
 * folded into that tool. Blank lines or non-continuation content close the
 * current tool. Inside fenced code blocks (```…```) we pass lines through
 * unchanged so we don't mangle snippets that happen to start with one of
 * our sentinel characters.
 */
export function parseAgentOutput(raw: string): ParsedAgentOutput {
  // Strip gl1tch's own protocol markers (stats JSON, [wrote:] acks, `<>`).
  const proto = extractGlitchProtocol(raw);

  // Pull <brain>…</brain> blocks out next so multi-line content doesn't
  // confuse the line-based logic below.
  const { stripped, brains } = extractBrains(proto.stripped);

  const summary: string[] = [...proto.summary];
  const tools: ToolCall[] = [...proto.tools];
  const bodyLines: string[] = [];

  let inFence = false;
  let cur: ToolCall | null = null;

  const flushTool = () => {
    if (cur) {
      tools.push(cur);
      cur = null;
    }
  };

  const lines = stripped.split("\n");

  for (const rawLine of lines) {
    const trimmed = rawLine.trim();

    // Code fences pass through verbatim.
    if (trimmed.startsWith("```")) {
      flushTool();
      inFence = !inFence;
      bodyLines.push(rawLine);
      continue;
    }
    if (inFence) {
      bodyLines.push(rawLine);
      continue;
    }

    // New tool header — finish any in-flight one.
    const header = parseHeader(trimmed);
    if (header) {
      flushTool();
      cur = { ok: header.ok, label: header.label, tool: header.tool };
      continue;
    }

    // Continuation of an open tool: `|`/`│` for command, `└` for result.
    if (cur) {
      const cont = parseContinuation(rawLine);
      if (cont) {
        if (cont.kind === "cmd") {
          cur.command = cur.command ? cur.command + " " + cont.text : cont.text;
        } else {
          cur.result = cur.result ? cur.result + " " + cont.text : cont.text;
        }
        continue;
      }

      // Indented wrap of the most recent continuation — just an extension
      // of whatever field we're currently building. Empty lines close the
      // tool; non-indented content also closes it (and falls through).
      if (trimmed === "") {
        flushTool();
        continue;
      }
      const isIndented = /^\s/.test(rawLine);
      if (isIndented) {
        if (cur.result !== undefined) {
          cur.result += " " + trimmed;
        } else if (cur.command !== undefined) {
          cur.command += " " + trimmed;
        }
        continue;
      }

      // Otherwise: close the tool and reprocess the line as normal content.
      flushTool();
    }

    if (isSummaryLine(trimmed)) {
      summary.push(trimmed);
      continue;
    }

    bodyLines.push(rawLine);
  }
  flushTool();

  // Collapse leading/trailing blank lines from the body.
  while (bodyLines.length && bodyLines[0].trim() === "") bodyLines.shift();
  while (bodyLines.length && bodyLines[bodyLines.length - 1].trim() === "") bodyLines.pop();

  return {
    summary,
    tools,
    brains,
    body: rewriteShellHeadings(bodyLines.join("\n")),
  };
}

// Rewrite shell-style banner headings (`=== Title ===`) into markdown h3s
// so they render as proper section breaks instead of inline `===` noise.
const SHELL_HEADING_RE = /^={2,}\s+(.+?)\s+={2,}\s*$/gm;
function rewriteShellHeadings(body: string): string {
  return body.replace(SHELL_HEADING_RE, (_m, title) => `### ${title}`);
}

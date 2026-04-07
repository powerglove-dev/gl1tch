import { Brain, Settings } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { BrainActivity, BrainState } from "@/lib/types";
import { formatTime12 } from "@/lib/time";
import {
  ListCollectors,
  ReadCollectorsConfig,
  WriteCollectorsConfig,
  CollectorsConfigPath,
  RecentCollectorLogs,
} from "../../wailsjs/go/main/App";

interface LogEntry {
  time_ms: number;
  level: string;
  source: string;
  message: string;
  attrs?: string;
}

interface CollectorInfo {
  name: string;
  enabled: boolean;
  interval_ms: number;
  detail: string;
  source?: string;
  // Live activity stats from Elasticsearch — populated when the
  // brain loop's collectorTick has had time to poll.
  total_docs?: number;
  last_seen_ms?: number;
  // In-process collector run heartbeat — populated by the registry
  // each collector calls into after every poll cycle. Tells us
  // "the collector actually ran at this time" even when ES has
  // nothing new for that source (healthy quiet polls).
  last_run_ms?: number;
  last_run_indexed?: number;
  last_run_duration_ms?: number;
  last_run_error?: string;
}

interface CollectorPayload {
  anchor_ms: number;
  now_ms: number;
  collectors: CollectorInfo[];
}

interface Props {
  state: BrainState;
  detail: string;
  activity: BrainActivity[];
  onMarkRead: () => void;
  // The active workspace — used to scope the collectors list so the
  // popover shows per-workspace directories/git/github. Falls back to
  // observer.yaml when no workspace is active (fresh startup).
  activeWorkspaceId: string | null;
  activeWorkspaceTitle: string;
}

/**
 * Persistent brain indicator. Lives in the titlebar (top-right) and is the
 * single ambient status surface in the app — no other system pills.
 *
 * Visual states:
 *   idle       → steady dim glow
 *   collecting → slow pulse, slightly brighter
 *   analyzing  → faster pulse + color shift
 *   alert      → solid bright + unread dot badge
 *   error      → red tint
 *
 * Click opens a popover listing recent brain activity (alerts + check-ins),
 * newest first. Opening the popover marks all unread entries as read.
 */
export function BrainIndicator({
  state,
  detail,
  activity,
  onMarkRead,
  activeWorkspaceId,
  activeWorkspaceTitle,
}: Props) {
  const [open, setOpen] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  // Collectors are fetched on-demand when the popover opens, with a 1s
  // tick while open so the "next run" countdown stays live. Closed →
  // no work happens.
  const [collectors, setCollectors] = useState<CollectorInfo[]>([]);
  const [anchorMs, setAnchorMs] = useState<number>(0);
  const [configPath, setConfigPath] = useState<string>("");
  const [editorOpen, setEditorOpen] = useState(false);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [, setNowTick] = useState(0);

  const unreadCount = activity.filter((a) => a.unread).length;

  // Click-outside to close
  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      const t = e.target as Node;
      if (popoverRef.current?.contains(t)) return;
      if (buttonRef.current?.contains(t)) return;
      setOpen(false);
    }
    window.addEventListener("mousedown", onClick);
    return () => window.removeEventListener("mousedown", onClick);
  }, [open]);

  // Fetch collectors when the popover opens, then tick every second so
  // the "next in Xs" / "Xs ago" labels stay live. We also re-fetch the
  // collector list every 5s while open so deltas from the backend's
  // collector poll show up without the user having to close/reopen.
  //
  // The effect re-runs when activeWorkspaceId changes so switching
  // workspaces with the popover open immediately shows the new
  // workspace's collector set.
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    const wsId = activeWorkspaceId ?? "";
    function refresh() {
      ListCollectors(wsId).then((json: string) => {
        if (cancelled) return;
        try {
          const payload = JSON.parse(json) as CollectorPayload;
          setCollectors(payload.collectors ?? []);
          setAnchorMs(payload.anchor_ms ?? Date.now());
        } catch {
          setCollectors([]);
        }
      });
      RecentCollectorLogs(60).then((json: string) => {
        if (cancelled) return;
        try {
          setLogs((JSON.parse(json) as LogEntry[]) ?? []);
        } catch {
          setLogs([]);
        }
      });
    }
    refresh();
    CollectorsConfigPath().then((p: string) => {
      if (!cancelled) setConfigPath(p);
    });
    const tickId = window.setInterval(() => setNowTick((n) => n + 1), 1000);
    const refreshId = window.setInterval(refresh, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(tickId);
      window.clearInterval(refreshId);
    };
  }, [open, activeWorkspaceId]);

  function toggle() {
    const next = !open;
    setOpen(next);
    if (next && unreadCount > 0) onMarkRead();
  }

  const visual = visualForState(state);
  const tooltip =
    detail ||
    (state === "idle"
      ? "brain idle"
      : state === "collecting"
        ? "watching · collecting"
        : state === "analyzing"
          ? "analyzing recent activity"
          : state === "alert"
            ? `${unreadCount} unread · click to view`
            : "brain offline");

  return (
    <>
    {editorOpen && (
      <CollectorsConfigEditor
        configPath={configPath}
        onClose={() => setEditorOpen(false)}
      />
    )}
    <div
      style={{
        position: "relative",
        WebkitAppRegion: "no-drag",
      } as React.CSSProperties}
    >
      <button
        ref={buttonRef}
        onClick={toggle}
        title={tooltip}
        aria-label="brain"
        style={{
          background: "none",
          border: "none",
          cursor: "pointer",
          padding: 6,
          borderRadius: 8,
          display: "flex",
          alignItems: "center",
          position: "relative",
          color: visual.color,
          opacity: visual.opacity,
          animation: visual.animation,
          transition: "color 0.2s, opacity 0.2s",
        }}
      >
        <Brain size={16} strokeWidth={2} />
        {unreadCount > 0 && (
          <span
            style={{
              position: "absolute",
              top: 3,
              right: 3,
              minWidth: 8,
              height: 8,
              borderRadius: 999,
              background: "var(--red, #ff5555)",
              boxShadow: "0 0 6px rgba(255,85,85,0.7)",
            }}
          />
        )}
      </button>

      {open && (
        <div
          ref={popoverRef}
          style={{
            position: "absolute",
            top: "100%",
            right: 0,
            marginTop: 6,
            width: 460,
            maxHeight: 600,
            overflowY: "auto",
            background: "var(--bg-dark)",
            border: "1px solid var(--border)",
            borderRadius: 10,
            boxShadow: "0 10px 30px rgba(0,0,0,0.5)",
            zIndex: 1000,
            fontSize: 12,
          }}
        >
          <div
            style={{
              padding: "10px 14px",
              borderBottom: "1px solid var(--border)",
              display: "flex",
              alignItems: "center",
              gap: 8,
              color: "var(--fg)",
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: "0.06em",
              fontSize: 10,
            }}
          >
            <Brain size={12} style={{ color: visual.color }} />
            <span style={{ flex: 1 }}>brain · {state}</span>
            {detail && (
              <span
                style={{
                  fontWeight: 400,
                  textTransform: "none",
                  letterSpacing: 0,
                  color: "var(--fg-dim)",
                }}
              >
                {detail}
              </span>
            )}
          </div>

          {/* Collectors block — a single flat list scoped to the
              active workspace. Directories/git/github come from the
              workspace's SQLite row; claude/copilot/mattermost are
              process-wide but shown under every workspace because
              the brain's memory includes chat context. */}
          <SectionHeader
            label={
              activeWorkspaceTitle
                ? `collectors · ${activeWorkspaceTitle}`
                : "collectors"
            }
            actionLabel="edit"
            actionTitle={configPath || "edit observer.yaml"}
            onAction={() => setEditorOpen(true)}
          />
          {collectors.length === 0 ? (
            <div
              style={{
                padding: "10px 14px 14px",
                color: "var(--fg-dim)",
                fontSize: 11,
                fontStyle: "italic",
              }}
            >
              {activeWorkspaceId
                ? "No collectors enabled for this workspace."
                : "Pick a workspace to see its collectors."}
            </div>
          ) : (
            <div style={{ paddingBottom: 6 }}>
              {collectors.map((c) => (
                <CollectorRow key={c.name} info={c} anchorMs={anchorMs} />
              ))}
            </div>
          )}

          <SectionLabel>activity</SectionLabel>
          {activity.length === 0 ? (
            <div
              style={{
                padding: "10px 14px 16px",
                color: "var(--fg-dim)",
                fontStyle: "italic",
                fontSize: 11,
              }}
            >
              No activity yet — the brain will speak up when it has something
              for you.
            </div>
          ) : (
            <div>
              {activity.map((entry) => (
                <ActivityRow key={entry.id} entry={entry} />
              ))}
            </div>
          )}

          {/* Live collector logs — tail of the in-process slog ring
              buffer. Auto-refreshes alongside the collectors list. */}
          <SectionLabel>logs</SectionLabel>
          {logs.length === 0 ? (
            <div
              style={{
                padding: "10px 14px 16px",
                color: "var(--fg-dim)",
                fontStyle: "italic",
                fontSize: 11,
              }}
            >
              No log output yet.
            </div>
          ) : (
            <div
              style={{
                paddingBottom: 8,
                fontFamily:
                  "Berkeley Mono, JetBrains Mono, Fira Code, SF Mono, monospace",
                fontSize: 10,
                lineHeight: 1.5,
              }}
            >
              {logs.map((l, i) => (
                <LogRow key={l.time_ms + "-" + i} entry={l} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
    </>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <SectionHeader label={children} />;
}

function SectionHeader({
  label,
  actionLabel,
  actionTitle,
  onAction,
}: {
  label: React.ReactNode;
  actionLabel?: string;
  actionTitle?: string;
  onAction?: () => void;
}) {
  return (
    <div
      style={{
        padding: "12px 14px 4px",
        display: "flex",
        alignItems: "center",
        gap: 8,
        fontSize: 9,
        fontWeight: 700,
        textTransform: "uppercase",
        letterSpacing: "0.1em",
        color: "var(--fg-dim)",
        opacity: 0.85,
      }}
    >
      <span style={{ flex: 1 }}>{label}</span>
      {actionLabel && onAction && (
        <button
          onClick={onAction}
          title={actionTitle}
          style={{
            background: "none",
            border: "1px solid var(--border)",
            color: "var(--cyan)",
            cursor: "pointer",
            padding: "2px 8px",
            borderRadius: 5,
            fontSize: 9,
            fontWeight: 700,
            textTransform: "uppercase",
            letterSpacing: "0.08em",
            display: "flex",
            alignItems: "center",
            gap: 4,
          }}
        >
          <Settings size={9} />
          {actionLabel}
        </button>
      )}
    </div>
  );
}

function CollectorRow({
  info,
  anchorMs,
}: {
  info: CollectorInfo;
  anchorMs: number;
}) {
  // nextRunIn = interval - ((now - anchor) mod interval)
  const now = Date.now();
  const interval = Math.max(1, info.interval_ms);
  const elapsed = Math.max(0, now - anchorMs);
  const nextInMs = interval - (elapsed % interval);

  const total = info.total_docs ?? 0;
  const lastSeen = info.last_seen_ms ?? 0;
  const lastRun = info.last_run_ms ?? 0;
  const lastRunErr = info.last_run_error ?? "";
  // "Active" = collector ran in the last 60s OR ES indexed something
  // in the last 60s. We watch BOTH signals so a collector that runs
  // every 5min still pulses green when it actually runs.
  const lastActivity = Math.max(lastSeen, lastRun);
  const recentlyActive = lastActivity > 0 && now - lastActivity < 60_000;
  const dotColor = lastRunErr
    ? "var(--red)"
    : recentlyActive
      ? "var(--green)"
      : total > 0 || lastRun > 0
        ? "var(--cyan)"
        : "var(--fg-dim)";

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 2,
        padding: "6px 14px",
        fontSize: 11,
        // Disabled collectors (e.g. the workspace section on a fresh
        // install) stay visible but dimmed so users can see what's
        // possible without having to dig into the config.
        opacity: info.enabled ? 1 : 0.5,
      }}
      title={info.source || info.detail}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <span
          style={{
            width: 5,
            height: 5,
            borderRadius: 999,
            background: dotColor,
            flexShrink: 0,
            boxShadow: recentlyActive ? `0 0 5px ${dotColor}` : "none",
            animation: recentlyActive ? "pulse 1.6s ease-in-out infinite" : "none",
          }}
        />
        <span style={{ color: "var(--fg)", fontWeight: 600, minWidth: 78 }}>
          {info.name}
        </span>
        <span
          style={{
            flex: 1,
            color: "var(--fg-dim)",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {info.detail}
        </span>
        <span
          style={{
            color: info.enabled ? "var(--cyan)" : "var(--fg-dim)",
            fontVariantNumeric: "tabular-nums",
            fontSize: 10,
          }}
        >
          {info.enabled ? `next in ${formatDuration(nextInMs)}` : "disabled"}
        </span>
      </div>
      {/* Activity stats — show whatever we have. ES counts come from
          the brain's collectorTick; the run heartbeat comes from the
          in-process registry each collector calls into. */}
      {(total > 0 || lastRun > 0 || lastRunErr) && (
        <div
          style={{
            display: "flex",
            gap: 10,
            paddingLeft: 15,
            color: "var(--fg-dim)",
            fontSize: 10,
            opacity: 0.85,
            flexWrap: "wrap",
          }}
        >
          {total > 0 && (
            <span style={{ fontVariantNumeric: "tabular-nums" }}>
              {total.toLocaleString()} indexed
            </span>
          )}
          {lastRun > 0 && (
            <span style={{ fontVariantNumeric: "tabular-nums" }}>
              · ran {formatRelative(lastRun)}
              {info.last_run_indexed != null && info.last_run_indexed > 0 && (
                <> · +{info.last_run_indexed} new</>
              )}
            </span>
          )}
          {lastRunErr && (
            <span
              style={{
                color: "var(--red)",
                opacity: 0.95,
                fontFamily: "monospace",
              }}
              title={lastRunErr}
            >
              · {truncate(lastRunErr, 40)}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return s.slice(0, n - 1) + "…";
}

// LogRow renders one captured slog record as a tight single-line
// entry. Source-tagged rows get a colored badge so the user can
// scan-read for a specific collector quickly.
function LogRow({ entry }: { entry: LogEntry }) {
  const levelColor =
    entry.level === "ERROR"
      ? "var(--red)"
      : entry.level === "WARN"
        ? "var(--yellow)"
        : "var(--cyan)";
  const sourceColor = sourceColorFor(entry.source);
  return (
    <div
      style={{
        display: "flex",
        gap: 6,
        padding: "2px 14px",
        color: "var(--fg)",
        opacity: 0.92,
        whiteSpace: "nowrap",
        overflow: "hidden",
      }}
      title={`${formatTime12(entry.time_ms)} ${entry.level} ${entry.source ? "[" + entry.source + "] " : ""}${entry.message}${entry.attrs ? " · " + entry.attrs : ""}`}
    >
      <span
        style={{
          color: "var(--fg-dim)",
          opacity: 0.6,
          fontVariantNumeric: "tabular-nums",
          flexShrink: 0,
        }}
      >
        {formatTime12(entry.time_ms)}
      </span>
      {entry.source && (
        <span
          style={{
            color: sourceColor,
            fontWeight: 600,
            flexShrink: 0,
            minWidth: 70,
          }}
        >
          {entry.source}
        </span>
      )}
      <span
        style={{
          color: levelColor,
          opacity: 0.7,
          flexShrink: 0,
          width: 32,
        }}
      >
        {entry.level.toLowerCase()}
      </span>
      <span
        style={{
          flex: 1,
          overflow: "hidden",
          textOverflow: "ellipsis",
        }}
      >
        {entry.message}
        {entry.attrs && (
          <span style={{ color: "var(--fg-dim)", marginLeft: 6 }}>
            {entry.attrs}
          </span>
        )}
      </span>
    </div>
  );
}

// sourceColorFor picks a stable Dracula-palette color per collector
// name so the user can scan the log stream for a specific source
// without reading the badge text.
function sourceColorFor(source: string): string {
  if (!source) return "var(--fg-dim)";
  const palette = [
    "var(--cyan)",
    "var(--green)",
    "var(--purple)",
    "var(--yellow)",
    "var(--orange)",
    "var(--pink, #ff79c6)",
  ];
  let hash = 0;
  for (let i = 0; i < source.length; i++) {
    hash = (hash * 31 + source.charCodeAt(i)) | 0;
  }
  return palette[Math.abs(hash) % palette.length];
}

function formatDuration(ms: number): string {
  if (ms <= 0) return "now";
  const s = Math.ceil(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const r = s % 60;
  if (m < 60) return r ? `${m}m ${r}s` : `${m}m`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

function ActivityRow({ entry }: { entry: BrainActivity }) {
  const sevColor =
    entry.severity === "error"
      ? "var(--red, #ff5555)"
      : entry.severity === "warn"
        ? "var(--yellow)"
        : "var(--cyan)";
  return (
    <div
      style={{
        padding: "10px 14px",
        borderBottom: "1px solid var(--border)",
        display: "flex",
        gap: 10,
        alignItems: "flex-start",
      }}
    >
      <div
        style={{
          width: 6,
          height: 6,
          borderRadius: 999,
          background: sevColor,
          marginTop: 6,
          flexShrink: 0,
          boxShadow: entry.unread ? `0 0 6px ${sevColor}` : "none",
        }}
      />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div
          style={{
            display: "flex",
            alignItems: "baseline",
            gap: 8,
            color: "var(--fg)",
            fontWeight: entry.unread ? 600 : 500,
          }}
        >
          <span
            style={{
              flex: 1,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {entry.title}
          </span>
          <span
            style={{ fontSize: 10, color: "var(--fg-dim)" }}
            title={formatRelative(entry.timestamp)}
          >
            {formatTime12(entry.timestamp)}
          </span>
        </div>
        {entry.detail && (
          <div
            style={{
              marginTop: 3,
              color: "var(--fg-dim)",
              fontSize: 11,
              lineHeight: 1.4,
            }}
          >
            {entry.detail}
          </div>
        )}
        {entry.source && (
          <div
            style={{
              marginTop: 4,
              color: "var(--fg-dim)",
              opacity: 0.7,
              fontSize: 10,
              fontFamily: "monospace",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {entry.source}
          </div>
        )}
      </div>
    </div>
  );
}

function visualForState(state: BrainState): {
  color: string;
  opacity: number;
  animation: string;
} {
  switch (state) {
    case "idle":
      return { color: "var(--fg-dim)", opacity: 0.45, animation: "none" };
    case "collecting":
      return {
        color: "var(--cyan)",
        opacity: 0.85,
        animation: "brain-pulse-slow 2.2s ease-in-out infinite",
      };
    case "analyzing":
      return {
        color: "var(--purple)",
        opacity: 1,
        animation: "brain-pulse-fast 1.1s ease-in-out infinite",
      };
    case "alert":
      return { color: "var(--yellow)", opacity: 1, animation: "none" };
    case "error":
      return { color: "var(--red, #ff5555)", opacity: 0.9, animation: "none" };
  }
}

function formatRelative(ts: number): string {
  const diff = Date.now() - ts;
  if (diff < 60_000) return "just now";
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h`;
  return `${Math.floor(diff / 86_400_000)}d`;
}

/**
 * In-app editor for observer.yaml. Loads the file via the Wails
 * bridge, lets the user edit it in a textarea, and writes it back —
 * no system text editor involved. The backend validates the YAML
 * before saving so a typo can never corrupt the running config.
 */
function CollectorsConfigEditor({
  configPath,
  onClose,
}: {
  configPath: string;
  onClose: () => void;
}) {
  const [content, setContent] = useState<string>("");
  const [original, setOriginal] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>("");
  const [savedAt, setSavedAt] = useState<number>(0);

  useEffect(() => {
    let cancelled = false;
    ReadCollectorsConfig().then((s: string) => {
      if (cancelled) return;
      setContent(s);
      setOriginal(s);
      setLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  // Esc to close
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
      if ((e.metaKey || e.ctrlKey) && e.key === "s") {
        e.preventDefault();
        save();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [content, saving]);

  async function save() {
    if (saving) return;
    setSaving(true);
    setError("");
    const result: string = await WriteCollectorsConfig(content);
    setSaving(false);
    if (result) {
      setError(result);
      return;
    }
    setOriginal(content);
    setSavedAt(Date.now());
  }

  function revert() {
    setContent(original);
    setError("");
  }

  const dirty = content !== original;

  return (
    <div
      onClick={(e) => {
        // Click on backdrop closes; clicks inside the modal don't
        // bubble (handled by stopPropagation below).
        if (e.target === e.currentTarget) onClose();
      }}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.55)",
        backdropFilter: "blur(3px)",
        zIndex: 9999,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 24,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          width: "min(820px, 100%)",
          maxHeight: "calc(100vh - 48px)",
          background: "var(--bg-dark)",
          border: "1px solid var(--border)",
          borderRadius: 12,
          boxShadow: "0 20px 60px rgba(0,0,0,0.6)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        {/* Header */}
        <div
          style={{
            padding: "14px 18px",
            borderBottom: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            gap: 12,
          }}
        >
          <Brain size={14} style={{ color: "var(--cyan)" }} />
          <div style={{ flex: 1 }}>
            <div
              style={{
                fontSize: 11,
                fontWeight: 700,
                textTransform: "uppercase",
                letterSpacing: "0.08em",
                color: "var(--fg)",
              }}
            >
              configure collectors
            </div>
            {configPath && (
              <div
                style={{
                  fontSize: 10,
                  color: "var(--fg-dim)",
                  marginTop: 2,
                  fontFamily: "monospace",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
                title={configPath}
              >
                {configPath}
              </div>
            )}
          </div>
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              color: "var(--fg-dim)",
              cursor: "pointer",
              fontSize: 18,
              padding: "4px 8px",
            }}
            title="Close (Esc)"
          >
            ×
          </button>
        </div>

        {/* Body */}
        <div
          style={{
            flex: 1,
            display: "flex",
            flexDirection: "column",
            minHeight: 0,
            padding: 14,
            gap: 10,
          }}
        >
          {loading ? (
            <div
              style={{
                padding: 30,
                textAlign: "center",
                color: "var(--fg-dim)",
                fontSize: 12,
              }}
            >
              loading config…
            </div>
          ) : (
            <textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              spellCheck={false}
              style={{
                flex: 1,
                minHeight: 360,
                fontFamily:
                  "Berkeley Mono, JetBrains Mono, Fira Code, SF Mono, monospace",
                fontSize: 12,
                lineHeight: 1.5,
                color: "var(--fg)",
                background: "var(--bg)",
                border: "1px solid var(--border)",
                borderRadius: 8,
                padding: 12,
                outline: "none",
                resize: "none",
                tabSize: 2,
              }}
            />
          )}
          {error && (
            <div
              style={{
                padding: "8px 12px",
                borderRadius: 6,
                background: "rgba(247,118,142,0.12)",
                border: "1px solid var(--red, #f7768e)",
                color: "var(--red, #f7768e)",
                fontSize: 11,
                fontFamily: "monospace",
                whiteSpace: "pre-wrap",
              }}
            >
              {error}
            </div>
          )}
        </div>

        {/* Footer */}
        <div
          style={{
            padding: "12px 18px",
            borderTop: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            gap: 10,
            background: "var(--bg-dark)",
          }}
        >
          <div
            style={{
              flex: 1,
              fontSize: 10,
              color: "var(--fg-dim)",
            }}
          >
            {dirty
              ? "unsaved changes · ⌘S to save · esc to close"
              : savedAt
                ? `saved ${formatTime12(savedAt)}`
                : "no changes"}
          </div>
          {dirty && (
            <button
              onClick={revert}
              style={{
                background: "none",
                border: "1px solid var(--border)",
                color: "var(--fg-dim)",
                padding: "6px 12px",
                borderRadius: 6,
                cursor: "pointer",
                fontSize: 11,
              }}
            >
              revert
            </button>
          )}
          <button
            onClick={save}
            disabled={!dirty || saving}
            style={{
              background: dirty ? "var(--cyan)" : "var(--bg-surface)",
              color: dirty ? "var(--bg-dark)" : "var(--fg-dim)",
              border: "1px solid " + (dirty ? "var(--cyan)" : "var(--border)"),
              padding: "6px 14px",
              borderRadius: 6,
              cursor: dirty && !saving ? "pointer" : "default",
              fontSize: 11,
              fontWeight: 600,
            }}
          >
            {saving ? "saving…" : "save"}
          </button>
        </div>
      </div>
    </div>
  );
}

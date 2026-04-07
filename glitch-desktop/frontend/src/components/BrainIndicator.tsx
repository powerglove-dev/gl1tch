import { Brain, Settings, Pencil, Plus, ScrollText, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { BrainActivity, BrainState } from "@/lib/types";
import { formatTime12 } from "@/lib/time";
import {
  ListCollectors,
  CollectorsConfigPath,
  RecentCollectorLogs,
  BrainDecisions,
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

// DecisionsSnapshot mirrors glitchd.BrainDecisionsActivity. Populated
// from the BrainDecisions Wails method on every popover open and on
// the same 5s tick that refreshes the collectors list. Any field can
// be missing when there are no decisions yet — the renderer treats
// zero values as "no activity" rather than an error.
interface DecisionsSnapshot {
  total: number;
  escalated: number;
  last_decision_ms?: number;
  last_provider?: string;
  last_escalated?: boolean;
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
  /** Open the EditorPopup on the active workspace's collectors.yaml.
   *  Plumbed up to App.tsx so the popup lives at the root and can
   *  share its CodeMirror + chat-refine surface with the rest of
   *  the editor flows. */
  onEditCollectors: () => void;
  /** Open the structured config modal for a specific collector (or
   *  the first one when no id is given). Plumbed up to App.tsx so
   *  the modal lives at the root alongside EditorPopup. */
  onConfigureCollector: (collectorId?: string) => void;
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
  onEditCollectors,
  onConfigureCollector,
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
  const [logs, setLogs] = useState<LogEntry[]>([]);
  // Drawer toggle for the focused logs view. The popover already
  // renders an inline logs section at the bottom, but on a popover
  // with claude/copilot/etc taking up most of the visible space the
  // logs scroll below the fold and users miss them — making "are
  // the collectors actually running" much harder to answer than it
  // should be. The drawer is the dedicated full-height view that
  // surfaces only the captured slog ring buffer.
  const [showLogsDrawer, setShowLogsDrawer] = useState(false);
  // Drawer logs are pulled with a higher limit than the inline
  // section because users usually open it specifically to debug
  // something and want enough context to see the full pod-startup
  // sequence (~30+ lines for a typical workspace cold-start).
  const [drawerLogs, setDrawerLogs] = useState<LogEntry[]>([]);
  // Brain decisions snapshot, refreshed alongside the collectors list.
  // null until the first fetch returns; the renderer treats null as
  // "loading" and zero-valued fields as "no activity yet".
  const [decisions, setDecisions] = useState<DecisionsSnapshot | null>(null);
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

  // Logs drawer fetch loop. Runs only when the drawer is open so we
  // don't pay the Wails round-trip cost on every popover interaction
  // for users who never open the drawer. Pulls the deeper limit
  // (300) the drawer wants vs the 60 the inline section uses, and
  // refreshes once a second so collector activity scrolls in live.
  useEffect(() => {
    if (!showLogsDrawer) return;
    let cancelled = false;
    function refresh() {
      RecentCollectorLogs(300).then((json: string) => {
        if (cancelled) return;
        try {
          setDrawerLogs((JSON.parse(json) as LogEntry[]) ?? []);
        } catch {
          setDrawerLogs([]);
        }
      });
    }
    refresh();
    const id = window.setInterval(refresh, 1000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [showLogsDrawer]);

  // Esc to close the drawer (mirrors the popover's own close
  // semantics — Esc collapses the topmost overlay first).
  useEffect(() => {
    if (!showLogsDrawer) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        setShowLogsDrawer(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [showLogsDrawer]);

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
      // Brain decisions snapshot (total / escalated / last). Same
      // workspace scoping as the collectors list. Errors collapse to
      // the empty snapshot — decisions data is informational, never
      // load-bearing for the rest of the popover.
      BrainDecisions(wsId).then((json: string) => {
        if (cancelled) return;
        try {
          const snap = JSON.parse(json) as DecisionsSnapshot;
          setDecisions(snap);
        } catch {
          setDecisions({ total: 0, escalated: 0 });
        }
      });
    }
    refresh();
    // Per the workspace-scoped collectors split: each workspace has
    // its own collectors.yaml. Empty workspace id falls back to the
    // global file (still useful at startup before a workspace is
    // active).
    CollectorsConfigPath(activeWorkspaceId ?? "").then((p: string) => {
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
    {/* Editor lives at the root in App.tsx now — the brain popover
        only emits the request via onEditCollectors. */}
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
            actionLabel="add"
            actionIcon="plus"
            actionTitle="Configure collectors"
            onAction={() => onConfigureCollector(undefined)}
            secondaryActionLabel="raw"
            secondaryActionTitle={configPath || "edit observer.yaml"}
            onSecondaryAction={onEditCollectors}
            tertiaryActionLabel="logs"
            tertiaryActionTitle="Show captured collector logs"
            onTertiaryAction={() => setShowLogsDrawer(true)}
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
                <CollectorRow
                  key={c.name}
                  info={c}
                  anchorMs={anchorMs}
                  onEdit={() => onConfigureCollector(c.name)}
                />
              ))}
            </div>
          )}

          {/* Aggregate stats so the user can see at a glance that the
              brain is doing work, even when no individual collector
              has tripped activity. Computed across the same collector
              payload the rows above use, so totals always match. */}
          <CollectorStats collectors={collectors} logs={logs} />

          {/* Brain decisions — counts of "the brain picked a chain"
              events scoped to the active workspace. Shows whether the
              brain is staying on the local Ollama or escalating to a
              paid model. Mirrors the data backing the Kibana dashboard
              so the popover and Lens charts always agree. */}
          <SectionLabel>decisions</SectionLabel>
          <DecisionsSection snapshot={decisions} />

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
    {showLogsDrawer && (
      <LogsDrawer
        logs={drawerLogs}
        onClose={() => setShowLogsDrawer(false)}
      />
    )}
    </>
  );
}

// LogsDrawer is the focused captured-log view that opens when the
// user clicks the "logs" button in the collectors header. It's a
// modal-style overlay rather than an inline expansion of the
// existing logs section because the popover's maxHeight=600 +
// internal scroll buries inline content below the fold on most
// workspaces — users open this drawer specifically to debug, and
// the answer they need (was a collector started? did it panic?
// did it index N docs?) is hard to find by scrolling a popover.
//
// The drawer takes its own click-outside / Esc / X-button close
// handling so it doesn't fight with the popover's outside-click
// listener (which would otherwise close BOTH on a single click).
// Stop event propagation on the inner panel keeps the popover
// open underneath when the user clicks inside the drawer.
function LogsDrawer({
  logs,
  onClose,
}: {
  logs: LogEntry[];
  onClose: () => void;
}) {
  return (
    <div
      onClick={(e) => {
        // Click on the dimmed backdrop closes; clicks inside the
        // panel are stopped below so they never reach this handler.
        if (e.target === e.currentTarget) onClose();
      }}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.65)",
        backdropFilter: "blur(3px)",
        zIndex: 9500, // above the brain popover (1000) and editor popup (9000)
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 32,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          width: "min(880px, 100%)",
          height: "min(640px, calc(100vh - 64px))",
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
            padding: "12px 18px",
            borderBottom: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            gap: 12,
            color: "var(--fg)",
            fontSize: 11,
            fontWeight: 700,
            textTransform: "uppercase",
            letterSpacing: "0.08em",
          }}
        >
          <ScrollText size={12} style={{ color: "var(--cyan)" }} />
          <span style={{ flex: 1 }}>captured collector logs</span>
          <span
            style={{
              fontWeight: 400,
              textTransform: "none",
              letterSpacing: 0,
              color: "var(--fg-dim)",
              fontSize: 10,
            }}
          >
            {logs.length} entries · live
          </span>
          <button
            type="button"
            onClick={onClose}
            title="Close (Esc)"
            style={{
              background: "none",
              border: "none",
              color: "var(--fg-dim)",
              cursor: "pointer",
              padding: 4,
              display: "flex",
            }}
          >
            <X size={14} />
          </button>
        </div>

        {/* Body */}
        <div
          style={{
            flex: 1,
            minHeight: 0,
            overflow: "auto",
            background: "var(--bg)",
            fontFamily:
              "Berkeley Mono, JetBrains Mono, Fira Code, SF Mono, monospace",
            fontSize: 11,
            lineHeight: 1.6,
            padding: "10px 0",
          }}
        >
          {logs.length === 0 ? (
            <div
              style={{
                padding: "40px 18px",
                color: "var(--fg-dim)",
                fontStyle: "italic",
                textAlign: "center",
              }}
            >
              No log output yet. The slog tee captures records from
              every collector goroutine in this process — if this
              stays empty after you've waited a minute or two, the
              workspace pod isn't starting any collectors.
            </div>
          ) : (
            logs.map((l, i) => (
              <LogRow key={l.time_ms + "-" + i} entry={l} />
            ))
          )}
        </div>

        {/* Footer */}
        <div
          style={{
            padding: "8px 18px",
            borderTop: "1px solid var(--border)",
            background: "var(--bg-dark)",
            display: "flex",
            alignItems: "center",
            gap: 8,
            fontSize: 10,
            color: "var(--fg-dim)",
          }}
        >
          <span style={{ flex: 1 }}>
            esc to close · refreshes every 1s while open
          </span>
        </div>
      </div>
    </div>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <SectionHeader label={children} />;
}

function SectionHeader({
  label,
  actionLabel,
  actionIcon = "settings",
  actionTitle,
  onAction,
  secondaryActionLabel,
  secondaryActionTitle,
  onSecondaryAction,
  tertiaryActionLabel,
  tertiaryActionTitle,
  onTertiaryAction,
}: {
  label: React.ReactNode;
  actionLabel?: string;
  actionIcon?: "settings" | "plus";
  actionTitle?: string;
  onAction?: () => void;
  /** Optional second button rendered to the LEFT of the primary
   *  action. The collectors header uses this to expose the raw-YAML
   *  escape hatch alongside the primary "add/configure" action. */
  secondaryActionLabel?: string;
  secondaryActionTitle?: string;
  onSecondaryAction?: () => void;
  /** Optional third button rendered to the LEFT of the secondary
   *  action. Used for the collectors header's "logs" pop-out which
   *  opens a focused log drawer over the popover — the existing
   *  in-line logs section at the bottom of the popover gets buried
   *  below the fold on most workspaces, so a button that scopes the
   *  view to nothing-but-logs is the actual surface users want when
   *  they're trying to figure out why a collector isn't running. */
  tertiaryActionLabel?: string;
  tertiaryActionTitle?: string;
  onTertiaryAction?: () => void;
}) {
  function renderButton(
    label: string,
    title: string | undefined,
    onClick: () => void,
    icon: "settings" | "plus" | "none",
  ) {
    return (
      <button
        onClick={onClick}
        title={title}
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
        {icon === "settings" && <Settings size={9} />}
        {icon === "plus" && <Plus size={9} />}
        {label}
      </button>
    );
  }
  return (
    <div
      style={{
        padding: "12px 14px 4px",
        display: "flex",
        alignItems: "center",
        gap: 6,
        fontSize: 9,
        fontWeight: 700,
        textTransform: "uppercase",
        letterSpacing: "0.1em",
        color: "var(--fg-dim)",
        opacity: 0.85,
      }}
    >
      <span style={{ flex: 1 }}>{label}</span>
      {tertiaryActionLabel &&
        onTertiaryAction &&
        renderButton(
          tertiaryActionLabel,
          tertiaryActionTitle,
          onTertiaryAction,
          "none",
        )}
      {secondaryActionLabel &&
        onSecondaryAction &&
        renderButton(
          secondaryActionLabel,
          secondaryActionTitle,
          onSecondaryAction,
          "none",
        )}
      {actionLabel &&
        onAction &&
        renderButton(actionLabel, actionTitle, onAction, actionIcon)}
    </div>
  );
}

function CollectorRow({
  info,
  anchorMs,
  onEdit,
}: {
  info: CollectorInfo;
  anchorMs: number;
  /** Open the structured config modal for this collector. The
   *  callback is wired up to the App-level modal so the popover
   *  doesn't have to own modal state. */
  onEdit?: () => void;
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
        {onEdit && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onEdit();
            }}
            title="Configure"
            style={{
              background: "none",
              border: "none",
              color: "var(--fg-dim)",
              cursor: "pointer",
              padding: 2,
              display: "flex",
              alignItems: "center",
            }}
          >
            <Pencil size={10} />
          </button>
        )}
      </div>
      {/* Activity stats — always shown for enabled collectors so the
          user can immediately tell whether each source is contributing
          to THIS workspace. The fields are sourced from two paths:
          ES counts (total, lastSeen) come from QueryCollectorActivityScoped,
          which now filters by workspace_id; the run heartbeat
          (lastRun, lastRunIndexed) comes from the in-process registry
          each collector calls into after every poll cycle.
          For disabled collectors we keep the old behavior of hiding
          the row entirely so the popover doesn't get noisy. */}
      {info.enabled && (
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
          <span style={{ fontVariantNumeric: "tabular-nums" }}>
            {total > 0 ? `${total.toLocaleString()} indexed` : "0 indexed"}
          </span>
          {lastSeen > 0 && (
            <span
              style={{ fontVariantNumeric: "tabular-nums" }}
              title={new Date(lastSeen).toLocaleString()}
            >
              · last seen {formatRelative(lastSeen)}
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

// CollectorStats summarises the live collector payload into a few
// glanceable numbers. The brain row already shows per-collector
// status, but this aggregate view answers "is anything actually
// happening?" without having to scan every row.
//
// Sources:
//   - total docs across all collectors → from ES doc-count snapshots
//   - active collectors → enabled count vs configured count
//   - last successful run + window of runs in the last hour → from
//     the in-process run heartbeat
//   - error count → number of collectors whose most recent run failed
// DecisionsSection renders the brain-decisions snapshot — total
// chains the brain has routed, how many escalated to a paid model,
// and the most recent decision's provider/timestamp. Mirrors the
// data backing the Kibana dashboard so the popover and Lens charts
// always agree on the numbers.
//
// Visual rules:
//   - escalation rate colored green at 0%, cyan up to 25%, red above
//   - "last decision" cell pulses cyan when fresh (< 60s) so the user
//     can tell if a chain just ran without watching the activity feed
//   - null snapshot (still loading) shows a thin placeholder grid
//     instead of jumping to "0 decisions" so the section doesn't
//     flicker on workspace switch
function DecisionsSection({ snapshot }: { snapshot: DecisionsSnapshot | null }) {
  if (snapshot == null) {
    return (
      <div
        style={{
          padding: "4px 14px 10px",
          fontSize: 11,
          color: "var(--fg-dim)",
          fontStyle: "italic",
        }}
      >
        loading…
      </div>
    );
  }
  const total = snapshot.total ?? 0;
  const escalated = snapshot.escalated ?? 0;
  const local = total - escalated;
  const escalationPct = total > 0 ? Math.round((escalated / total) * 100) : 0;
  const escalationColor =
    total === 0
      ? "var(--fg-dim)"
      : escalationPct === 0
        ? "var(--green)"
        : escalationPct < 25
          ? "var(--cyan)"
          : "var(--red, #ff5555)";

  const lastMs = snapshot.last_decision_ms ?? 0;
  const lastFresh = lastMs > 0 && Date.now() - lastMs < 60 * 1000;
  const lastLabel =
    lastMs > 0
      ? `${snapshot.last_provider || "?"} · ${formatRelative(lastMs)}`
      : "—";

  return (
    <div
      style={{
        padding: "4px 14px 10px",
        display: "grid",
        gridTemplateColumns: "1fr 1fr",
        rowGap: 6,
        columnGap: 12,
        fontSize: 11,
        color: "var(--fg)",
      }}
    >
      <StatRow
        label="total"
        value={total.toLocaleString()}
        color={total > 0 ? "var(--cyan)" : undefined}
      />
      <StatRow
        label="escalated · paid"
        value={
          total > 0
            ? `${escalated} (${escalationPct}%)`
            : escalated.toString()
        }
        color={escalationColor}
      />
      <StatRow
        label="local · ollama"
        value={local.toLocaleString()}
        color={local > 0 ? "var(--green)" : undefined}
      />
      <StatRow
        label="last decision"
        value={lastLabel}
        color={
          lastFresh
            ? snapshot.last_escalated
              ? "var(--red, #ff5555)"
              : "var(--green)"
            : undefined
        }
      />
    </div>
  );
}

function CollectorStats({
  collectors,
  logs,
}: {
  collectors: CollectorInfo[];
  logs: LogEntry[];
}) {
  const now = Date.now();
  let totalDocs = 0;
  let enabled = 0;
  let runsLastHour = 0;
  let errors = 0;
  let lastRun = 0;
  for (const c of collectors) {
    totalDocs += c.total_docs ?? 0;
    if (c.enabled) enabled++;
    if (c.last_run_ms && c.last_run_ms > 0) {
      if (now - c.last_run_ms < 60 * 60 * 1000) runsLastHour++;
      if (c.last_run_ms > lastRun) lastRun = c.last_run_ms;
    }
    if (c.last_run_error) errors++;
  }
  // Recent log activity gives a "still alive in the last minute"
  // pulse signal independent of collector heartbeats — useful when
  // a collector is busy mid-cycle and hasn't reported a run yet.
  const recentLogs = logs.filter((l) => now - l.time_ms < 60 * 1000).length;

  const healthColor =
    errors > 0
      ? "var(--red, #ff5555)"
      : runsLastHour > 0 || recentLogs > 0
        ? "var(--green)"
        : enabled > 0
          ? "var(--cyan)"
          : "var(--fg-dim)";

  return (
    <>
      <SectionLabel>stats</SectionLabel>
      <div
        style={{
          padding: "4px 14px 10px",
          display: "grid",
          gridTemplateColumns: "1fr 1fr",
          rowGap: 6,
          columnGap: 12,
          fontSize: 11,
          color: "var(--fg)",
        }}
      >
        <StatRow
          label="status"
          value={
            errors > 0
              ? `${errors} error${errors === 1 ? "" : "s"}`
              : runsLastHour > 0 || recentLogs > 0
                ? "active"
                : enabled > 0
                  ? "idle"
                  : "no collectors"
          }
          color={healthColor}
        />
        <StatRow
          label="enabled"
          value={`${enabled} / ${collectors.length}`}
        />
        <StatRow
          label="total indexed"
          value={totalDocs.toLocaleString()}
          color={totalDocs > 0 ? "var(--cyan)" : undefined}
        />
        <StatRow
          label="runs · last hour"
          value={runsLastHour.toString()}
          color={runsLastHour > 0 ? "var(--green)" : undefined}
        />
        <StatRow
          label="last run"
          value={lastRun > 0 ? formatRelative(lastRun) : "—"}
        />
        <StatRow
          label="logs · last minute"
          value={recentLogs.toString()}
          color={recentLogs > 0 ? "var(--green)" : undefined}
        />
      </div>
    </>
  );
}

function StatRow({
  label,
  value,
  color,
}: {
  label: string;
  value: string;
  color?: string;
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "baseline",
        gap: 6,
        minWidth: 0,
      }}
    >
      <span
        style={{
          fontSize: 9,
          textTransform: "uppercase",
          letterSpacing: "0.06em",
          color: "var(--fg-dim)",
          opacity: 0.75,
          flexShrink: 0,
        }}
      >
        {label}
      </span>
      <span
        style={{
          fontVariantNumeric: "tabular-nums",
          fontWeight: 600,
          color: color ?? "var(--fg)",
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        {value}
      </span>
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


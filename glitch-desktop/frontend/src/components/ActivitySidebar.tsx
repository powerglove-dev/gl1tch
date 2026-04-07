/**
 * ActivitySidebar — the right-side ambient activity stream.
 *
 * Used to live as a dropdown inside the brain popover (see the
 * <SectionLabel>activity</SectionLabel> + activity.map block in
 * BrainIndicator.tsx). The dropdown was a hidden surface — users
 * had to click the brain icon, scroll past collectors / stats /
 * decisions, and only THEN find the activity entries — which
 * defeated the whole point of having an ambient activity feed.
 *
 * Lifting it into a right-side hideable sidebar mirrors the left
 * workspace sidebar's UX: default-open, persistent, toggleable
 * from the titlebar. The brain popover keeps showing collectors
 * / stats / decisions / logs (its diagnostic surfaces) and the
 * activity stream becomes the always-visible "what's the brain
 * up to" feed it was always supposed to be.
 *
 * The component is intentionally a thin wrapper around the same
 * BrainActivity rows the popover used. We keep the renderer in
 * sync visually with the rest of the popover — same colors,
 * same spacing — so users moving between surfaces don't see
 * style drift.
 */
import { Activity, X, Trash2 } from "lucide-react";
import type { BrainActivity } from "@/lib/types";
import { formatRelative, formatTime12 } from "@/lib/time";

interface Props {
  activity: BrainActivity[];
  /** Mark every activity entry as read. Wired to the same
   *  reducer action the brain popover used to call when it
   *  opened — keeps the unread badge logic working unchanged. */
  onMarkRead: () => void;
  /** Hide the sidebar. The titlebar's toggle button calls this
   *  too; the in-sidebar X is a convenience for users who want
   *  to dismiss it from the panel itself. */
  onClose: () => void;
  /** Optional clear-history hook. The brain popover doesn't
   *  expose this today (entries scroll out of the cap eventually),
   *  but the sidebar has the room for an explicit "clear" button
   *  so users with a long-running session can wipe noise without
   *  restarting. Wired iff the parent passes a handler. */
  onClear?: () => void;
}

export function ActivitySidebar({
  activity,
  onMarkRead,
  onClose,
  onClear,
}: Props) {
  const unreadCount = activity.filter((a) => a.unread).length;

  return (
    <div
      style={{
        width: 320,
        flexShrink: 0,
        background: "var(--bg-dark)",
        borderLeft: "1px solid var(--border)",
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
      }}
    >
      {/* Header — matches the left sidebar's header height + style
          so the two columns visually balance the chat in the middle.
          Activity icon + label on the left, unread badge in the middle,
          clear/close buttons on the right. */}
      <div
        style={{
          padding: "12px 14px",
          borderBottom: "1px solid var(--border)",
          display: "flex",
          alignItems: "center",
          gap: 8,
          color: "var(--fg)",
          fontWeight: 600,
          fontSize: 11,
          textTransform: "uppercase",
          letterSpacing: "0.08em",
        }}
      >
        <Activity size={12} style={{ color: "var(--cyan)" }} />
        <span style={{ flex: 1 }}>activity</span>
        {unreadCount > 0 && (
          <span
            style={{
              fontSize: 9,
              padding: "2px 6px",
              borderRadius: 10,
              background: "var(--cyan)",
              color: "var(--bg-dark)",
              fontWeight: 700,
              letterSpacing: 0,
            }}
            title={`${unreadCount} unread`}
          >
            {unreadCount}
          </span>
        )}
        {onClear && activity.length > 0 && (
          <button
            type="button"
            onClick={onClear}
            title="Clear all activity"
            style={{
              background: "none",
              border: "none",
              color: "var(--fg-dim)",
              cursor: "pointer",
              padding: 4,
              display: "flex",
            }}
          >
            <Trash2 size={11} />
          </button>
        )}
        <button
          type="button"
          onClick={onClose}
          title="Hide activity sidebar"
          style={{
            background: "none",
            border: "none",
            color: "var(--fg-dim)",
            cursor: "pointer",
            padding: 4,
            display: "flex",
          }}
        >
          <X size={12} />
        </button>
      </div>

      {/* Body — scrollable list of entries newest first. We mark
          read on first render of any unread entries by calling
          onMarkRead in a microtask once the user has actually seen
          the panel; that mirrors the brain popover's old "open
          marks read" semantics without needing an explicit click. */}
      <div
        style={{
          flex: 1,
          overflowY: "auto",
        }}
        // onMouseEnter is a softer "you've seen this" trigger than
        // a useEffect that fires on every render — moving the mouse
        // into the sidebar is a clear "I'm looking" signal, and it
        // avoids marking entries read just because the panel is
        // open in the background.
        onMouseEnter={() => {
          if (unreadCount > 0) onMarkRead();
        }}
      >
        {activity.length === 0 ? (
          <div
            style={{
              padding: "20px 14px",
              color: "var(--fg-dim)",
              fontSize: 11,
              fontStyle: "italic",
              lineHeight: 1.5,
            }}
          >
            No activity yet — the brain will speak up when collectors
            index something new or triage finds something worth
            flagging.
          </div>
        ) : (
          activity.map((entry) => <ActivityRow key={entry.id} entry={entry} />)
        )}
      </div>
    </div>
  );
}

// ActivityRow is a near-copy of the BrainIndicator popover's
// ActivityRow — same severity dot, same title/timestamp/detail/
// source layout. Lives here instead of being imported from the
// BrainIndicator file so the popover and the sidebar can evolve
// their item rendering independently if one needs richer affordances
// (e.g. click-to-expand, click-to-jump-to-source) without dragging
// the other along.
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
            fontSize: 12,
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

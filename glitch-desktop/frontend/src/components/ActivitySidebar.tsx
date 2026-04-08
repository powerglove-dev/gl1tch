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
import { Activity, X, Trash2, Sparkles, ChevronRight } from "lucide-react";
import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
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

// ActivityRow renders one entry in the activity stream. Three visual
// modes based on entry.kind:
//
//   - "checkin" / "alert": same compact layout the popover used to
//     have — severity dot, title, terse one-liner detail, source.
//   - "analysis": same row chrome PLUS a click-to-expand markdown
//     body. Collapsed by default; click anywhere on the row header
//     to expand. The expanded state shows the full markdown the
//     opencode-driven analyzer produced, plus a footer with the
//     model name, repo, and run duration.
//
// Lives here instead of being imported from BrainIndicator so the
// popover and the sidebar can evolve their item rendering
// independently. The popover's row stays compact; the sidebar's row
// can grow rich affordances like the analysis expand state without
// dragging the popover along.
function ActivityRow({ entry }: { entry: BrainActivity }) {
  const isAnalysis = entry.kind === "analysis";
  // Analysis rows expand on click. Collapsed by default so the
  // activity panel doesn't become a wall of text after a busy
  // collector tick — the user opts into the deep view per row.
  const [expanded, setExpanded] = useState(false);

  const sevColor = isAnalysis
    ? "var(--purple, #bd93f9)"
    : entry.severity === "error"
      ? "var(--red, #ff5555)"
      : entry.severity === "warn"
        ? "var(--yellow)"
        : "var(--cyan)";

  return (
    <div
      style={{
        padding: "10px 14px",
        borderBottom: "1px solid var(--border)",
        cursor: isAnalysis ? "pointer" : "default",
      }}
      onClick={isAnalysis ? () => setExpanded((v) => !v) : undefined}
    >
      <div style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
        {/* Severity dot — purple for analysis, severity-color for others */}
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
            {isAnalysis && (
              <Sparkles
                size={11}
                style={{ color: "var(--purple, #bd93f9)", flexShrink: 0 }}
              />
            )}
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
            {isAnalysis && (
              <ChevronRight
                size={11}
                style={{
                  color: "var(--fg-dim)",
                  transform: expanded ? "rotate(90deg)" : "rotate(0)",
                  transition: "transform 0.15s",
                  flexShrink: 0,
                }}
              />
            )}
            <span
              style={{ fontSize: 10, color: "var(--fg-dim)" }}
              title={formatRelative(entry.timestamp)}
            >
              {formatTime12(entry.timestamp)}
            </span>
          </div>

          {/* Compact detail — only shown when NOT an expanded analysis.
              For analysis rows, the markdown body lives in the expanded
              section below; the collapsed state shows just the title +
              source so the row stays scannable. */}
          {!isAnalysis && entry.detail && (
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

          {(entry.source || entry.repo) && (
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
              {[entry.source, entry.repo].filter(Boolean).join(" · ")}
            </div>
          )}
        </div>
      </div>

      {/* Expanded markdown body for analysis rows. Indented to align
          with the title text under the severity dot. Uses the same
          ReactMarkdown + remark-gfm pipeline TextBlock uses so code
          blocks, lists, headings all render consistently. */}
      {isAnalysis && expanded && (
        <div
          style={{
            marginTop: 8,
            marginLeft: 16,
            padding: "10px 12px",
            background: "var(--bg)",
            border: "1px solid var(--border)",
            borderRadius: 6,
            fontSize: 11,
            lineHeight: 1.5,
            color: "var(--fg)",
          }}
          // Stop click bubbling so clicking inside the expanded body
          // (e.g. selecting text) doesn't collapse the row.
          onClick={(e) => e.stopPropagation()}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {entry.detail}
          </ReactMarkdown>
          <div
            style={{
              marginTop: 8,
              paddingTop: 6,
              borderTop: "1px solid var(--border)",
              fontSize: 9,
              color: "var(--fg-dim)",
              fontFamily: "monospace",
              display: "flex",
              gap: 8,
              flexWrap: "wrap",
            }}
          >
            {entry.model && <span>model: {entry.model}</span>}
            {entry.event_type && <span>type: {entry.event_type}</span>}
            {entry.duration_ms != null && (
              <span>took: {(entry.duration_ms / 1000).toFixed(1)}s</span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

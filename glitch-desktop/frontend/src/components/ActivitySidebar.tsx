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
import { Activity, X, Trash2, Sparkles, ChevronRight, ExternalLink } from "lucide-react";
import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { BrainActivity, ActivityItem } from "@/lib/types";
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
  /** Open the indexed-docs drill-in modal scoped to a source and
   *  optional time window. Called when the user clicks an
   *  indexing-kind row's "View all" affordance or an alert-kind
   *  row. The parent owns the modal so its state survives
   *  sidebar toggles.
   *
   *  preselectRefs is an optional list of sha/url strings the
   *  modal should mark as selected on load — used by the per-card
   *  Analyze button so clicking the button on a single item
   *  opens the modal with exactly that item already ticked and
   *  the analyzer primed on it. */
  onOpenIndexedDocs?: (
    source: string,
    sinceMs?: number,
    initialPrompt?: string,
    preselectRefs?: string[],
  ) => void;
}

export function ActivitySidebar({
  activity,
  onMarkRead,
  onClose,
  onClear,
  onOpenIndexedDocs,
}: Props) {
  const unreadCount = activity.filter((a) => a.unread).length;

  return (
    <div
      style={{
        width: 260,
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
          activity.map((entry) => (
            <ActivityRow
              key={entry.id}
              entry={entry}
              onOpenIndexedDocs={onOpenIndexedDocs}
            />
          ))
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
function ActivityRow({
  entry,
  onOpenIndexedDocs,
}: {
  entry: BrainActivity;
  onOpenIndexedDocs?: (
    source: string,
    sinceMs?: number,
    initialPrompt?: string,
    preselectRefs?: string[],
  ) => void;
}) {
  const isAnalysis = entry.kind === "analysis";
  // Indexing rows are checkin-kind (or alert-kind for large
  // deltas) that carry an `items` preview. They behave like
  // analysis rows — expandable on click — but reveal a list of
  // preview docs instead of a markdown body.
  const hasIndexingPreview =
    !isAnalysis && (entry.items?.length ?? 0) > 0 && !!entry.source;
  const isClickable = isAnalysis || hasIndexingPreview;
  // Analysis and indexing rows both expand on click. Collapsed
  // by default so the activity panel doesn't become a wall of
  // text after a busy collector tick — the user opts into the
  // deep view per row.
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
        cursor: isClickable ? "pointer" : "default",
      }}
      onClick={isClickable ? () => setExpanded((v) => !v) : undefined}
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
            {isClickable && (
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

      {/* Expanded preview for indexing rows — shows the items
          array the backend attaches to collector check-in events.
          Each item is one recently-indexed doc: title + source
          badge + timestamp. A "View all" / "Analyze" action row
          opens the drill-in modal scoped to this row's source and
          time window. */}
      {hasIndexingPreview && expanded && (
        <div
          style={{
            marginTop: 8,
            marginLeft: 16,
            padding: "6px 0",
            borderTop: "1px dashed var(--border)",
          }}
          onClick={(e) => e.stopPropagation()}
        >
          {(entry.items ?? []).map((it, i) => (
            <IndexingPreviewItem
              key={`${it.sha || it.url || i}`}
              item={it}
              onAnalyze={
                onOpenIndexedDocs && entry.source
                  ? () => {
                      const ref = it.sha || it.url;
                      if (!ref) return;
                      onOpenIndexedDocs(
                        entry.source!,
                        entry.window_from_ms && entry.window_from_ms > 0
                          ? entry.window_from_ms
                          : undefined,
                        undefined,
                        [ref],
                      );
                    }
                  : undefined
              }
            />
          ))}
          {onOpenIndexedDocs && entry.source && (
            <button
              type="button"
              onClick={() =>
                onOpenIndexedDocs(
                  entry.source!,
                  entry.window_from_ms && entry.window_from_ms > 0
                    ? entry.window_from_ms
                    : undefined,
                )
              }
              style={{
                marginTop: 6,
                width: "100%",
                background: "transparent",
                border: "1px solid var(--border)",
                color: "var(--cyan)",
                padding: "6px 10px",
                borderRadius: 6,
                fontSize: 10,
                cursor: "pointer",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                gap: 6,
                textTransform: "uppercase",
                letterSpacing: "0.06em",
                fontWeight: 600,
              }}
            >
              <ExternalLink size={10} /> View all &amp; analyze
            </button>
          )}
        </div>
      )}

      {/* Expanded markdown body for analysis rows. Indented to align
          with the title text under the severity dot. Uses the same
          ReactMarkdown + remark-gfm pipeline TextBlock uses so code
          blocks, lists, headings all render consistently. */}
      {isAnalysis && expanded && (
        <div
          key="analysis-body"
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

/**
 * IndexingPreviewItem renders one doc from the preview list the
 * backend attaches to collector indexing events. Tight one-line
 * layout so 5 of them fit comfortably under an expanded row
 * without the sidebar feeling crowded.
 */
function IndexingPreviewItem({
  item,
  onAnalyze,
}: {
  item: ActivityItem;
  /** Opens the drill-in modal with this one card pre-selected and
   *  the analyzer primed to run on it. When omitted, the Analyze
   *  affordance is hidden — used for items with no sha/url that
   *  can't be stably identified on the modal side. */
  onAnalyze?: () => void;
}) {
  return (
    <div
      style={{
        padding: "4px 6px",
        display: "flex",
        alignItems: "flex-start",
        gap: 6,
      }}
    >
      <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 2 }}>
        <div
          style={{
            color: "var(--fg)",
            fontSize: 11,
            lineHeight: 1.35,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
          title={item.title}
        >
          {item.title}
        </div>
        <div
          style={{
            color: "var(--fg-dim)",
            fontSize: 9,
            fontFamily: "monospace",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {[item.type, item.repo, item.author && `@${item.author}`]
            .filter(Boolean)
            .join(" · ")}
        </div>
      </div>
      {/* Per-card Analyze button. Clicking opens the drill-in modal
          with this single doc pre-selected and the analyzer primed
          to run on it — a one-click path from "I see this in the
          feed" to "tell me what it means" without making the user
          open the full list and hunt the row down. */}
      {onAnalyze && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onAnalyze();
          }}
          title="Analyze this document"
          style={{
            background: "transparent",
            border: "1px solid var(--border)",
            color: "var(--cyan)",
            padding: "3px 6px",
            borderRadius: 4,
            cursor: "pointer",
            display: "flex",
            alignItems: "center",
            gap: 3,
            fontSize: 9,
            flexShrink: 0,
          }}
          onMouseEnter={(e) => {
            (e.currentTarget as HTMLButtonElement).style.background =
              "rgba(139, 233, 253, 0.08)";
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLButtonElement).style.background = "transparent";
          }}
        >
          <Sparkles size={9} />
          analyze
        </button>
      )}
    </div>
  );
}

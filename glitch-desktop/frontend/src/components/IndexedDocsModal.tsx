/**
 * IndexedDocsModal — drill-in surface for the activity sidebar.
 *
 * Opens when the user clicks an indexing-kind activity row (e.g.
 * "claude · 8 new") or a "View all" affordance under the row's
 * inline preview. Shows every indexed document for that source +
 * time window in a scrollable list (left pane) alongside an
 * analysis workspace (right pane) where the user can run on-demand
 * LLM analyses over all docs or a selected subset, optionally with
 * a free-form question. Results stream in live via the
 * brain:analysis:stream Wails event and are persisted to
 * glitch-analyses as linked analysis activity rows.
 *
 * Design notes:
 *  - Big modal, not a narrow side panel. The activity sidebar is
 *    too skinny for raw JSON doc previews and streaming markdown
 *    side by side.
 *  - Dracula styling mirrors CollectorConfigModal so the two
 *    modals feel like siblings.
 *  - Selection is optional. "Analyze all" acts over the current
 *    list; "Analyze selected (N)" acts over the checkbox subset.
 *    An empty selection disables the selected button.
 *  - The analysis pane is reused across turns: first analysis,
 *    first refinement, second refinement, etc. Each refinement is
 *    an independent opencode run linked by parent_analysis_id —
 *    there is no session continuation (see design.md decision 6).
 *  - AI-first. No heuristics about which docs are "interesting"
 *    or which source types support analysis. Every doc is a
 *    first-class selectable unit; the LLM decides what matters.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { X, Play, Sparkles, ChevronDown, ChevronRight, RefreshCw, Square } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

import {
  ListIndexedDocs,
  AnalyzeActivityChunks,
  CancelActivityAnalysis,
} from "../../wailsjs/go/main/App";
import type { IndexedDoc } from "@/lib/types";
import {
  useAnalysisStream,
  startAnalysisStream,
  resetAnalysisStream,
} from "@/lib/analysisStreams";
import { formatTime12 } from "@/lib/time";

interface Props {
  source: string;
  /** Optional lower-bound ms timestamp to scope the doc query.
   *  The sidebar passes the previous poll's last_seen_ms so the
   *  modal opens pre-filtered to "docs indexed since last poll". */
  sinceMs?: number;
  /** Optional initial prompt — set when the modal opens from an
   *  alert row so the alert's hook pre-populates the prompt box. */
  initialPrompt?: string;
  /** Optional list of doc refs (sha or url) to pre-select when the
   *  list finishes loading. Used by per-card Analyze buttons on
   *  activity items: one click opens the modal with exactly that
   *  doc ticked and the analysis auto-kicks off so the user sees
   *  results immediately instead of re-picking the card they just
   *  clicked. */
  preselectRefs?: string[];
  onClose: () => void;
}

export function IndexedDocsModal({
  source,
  sinceMs,
  initialPrompt,
  preselectRefs,
  onClose,
}: Props) {
  // ── Doc list state ─────────────────────────────────────────────────
  const [docs, setDocs] = useState<IndexedDoc[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [selection, setSelection] = useState<Set<string>>(new Set());
  const [expandedDoc, setExpandedDoc] = useState<string | null>(null);

  // ── Analysis state ─────────────────────────────────────────────────
  const [prompt, setPrompt] = useState(initialPrompt ?? "");
  const [streamId, setStreamId] = useState("");
  const [parentAnalysisId, setParentAnalysisId] = useState("");
  const [followupPrompt, setFollowupPrompt] = useState("");
  const stream = useAnalysisStream(streamId);
  const analysisScrollRef = useRef<HTMLDivElement | null>(null);
  // Flag that tracks whether the one-shot preselect+auto-run hook
  // has already fired for this modal instance. We only want the
  // per-card "open and analyze" flow to run ONCE when the modal
  // first opens with preselectRefs, not every time the doc list
  // refreshes afterwards (refresh shouldn't silently re-kick a
  // paid LLM run).
  const didAutoRunRef = useRef(false);

  // ── Load documents ─────────────────────────────────────────────────
  const loadDocs = useCallback(() => {
    setLoading(true);
    setLoadError("");
    ListIndexedDocs(source, sinceMs ?? 0, 500)
      .then((raw) => {
        try {
          const parsed = JSON.parse(raw);
          if (parsed?.error) {
            setLoadError(String(parsed.error));
            setDocs([]);
            return;
          }
          const loaded: IndexedDoc[] = Array.isArray(parsed?.docs) ? parsed.docs : [];
          setDocs(loaded);
        } catch (e) {
          setLoadError(`parse failed: ${String(e)}`);
          setDocs([]);
        }
      })
      .catch((e) => setLoadError(String(e)))
      .finally(() => setLoading(false));
  }, [source, sinceMs]);

  useEffect(() => {
    loadDocs();
  }, [loadDocs]);

  // Auto-scroll the analysis pane as tokens land so the user
  // always sees the latest output without having to chase it.
  useEffect(() => {
    const el = analysisScrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [stream.text]);

  // ESC closes, Cmd+Enter runs the current action
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Clean up the stream buffer when the modal closes so a later
  // reopen on the same source doesn't inherit a stale transcript.
  useEffect(() => {
    return () => {
      if (streamId) resetAnalysisStream(streamId);
    };
  }, [streamId]);

  // ── Doc selection helpers ──────────────────────────────────────────
  // Stable per-doc id: prefer SHA, then URL, then a fallback hash
  // of source+timestamp+message. The modal only needs uniqueness
  // within the current list, not across sessions.
  const docId = useCallback((d: IndexedDoc): string => {
    if (d.sha) return d.sha;
    if (d.url) return d.url;
    return `${d.source}-${d.timestamp_ms ?? 0}-${(d.message ?? "").slice(0, 32)}`;
  }, []);

  const toggleSelect = useCallback(
    (d: IndexedDoc) => {
      const id = docId(d);
      setSelection((prev) => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
    },
    [docId],
  );

  const selectAll = useCallback(() => {
    setSelection(new Set(docs.map(docId)));
  }, [docs, docId]);

  const clearSelection = useCallback(() => {
    setSelection(new Set());
  }, []);

  // ── Analysis invocation ────────────────────────────────────────────
  // scope is "all" (no eventRefs filter) or "selected" (build
  // eventRefs from the current selection state). explicitRefs
  // bypasses both and sends exactly the refs the caller provides —
  // used by the per-card auto-run path which can't rely on
  // selection state being flushed before the call.
  const runAnalysis = useCallback(
    (scope: "all" | "selected", userPrompt: string, explicitRefs?: string[]) => {
      if (docs.length === 0) return;
      // Map the selection (or everything) to the eventRefs the
      // backend expects. Only SHAs and URLs are accepted refs —
      // if a selected doc has neither, it silently falls back to
      // "analyze all" because the backend can't filter to it.
      let refs: string[] = [];
      if (explicitRefs && explicitRefs.length > 0) {
        refs = explicitRefs;
      } else if (scope === "selected") {
        docs.forEach((d) => {
          const id = docId(d);
          if (selection.has(id)) {
            if (d.sha) refs.push(d.sha);
            else if (d.url) refs.push(d.url);
          }
        });
      }

      // New stream id per invocation so refinements don't clobber
      // the parent's buffer. We also clear the followup field so
      // the user doesn't see stale text after a run kicks off.
      const newId = `client-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      setStreamId(newId);
      setFollowupPrompt("");
      startAnalysisStream(newId);

      AnalyzeActivityChunks(
        JSON.stringify({
          source,
          sinceMs: sinceMs ?? 0,
          eventRefs: refs,
          userPrompt,
          parentAnalysisId,
        }),
      )
        .then((raw) => {
          try {
            const parsed = JSON.parse(raw);
            if (parsed?.error) {
              resetAnalysisStream(newId);
              setStreamId("");
              setLoadError(String(parsed.error));
              return;
            }
            // Capture the server-assigned stream id from the
            // response. The server's id is what
            // brain:analysis:stream events are keyed by, so we
            // must listen on THAT id, not our client-side one.
            if (parsed?.streamId && parsed.streamId !== newId) {
              // Move the "streaming" mark from the placeholder
              // id to the real one, otherwise the first events
              // land in a bucket nobody is watching.
              resetAnalysisStream(newId);
              startAnalysisStream(parsed.streamId);
              setStreamId(parsed.streamId);
            }
          } catch {
            // Malformed response — leave the placeholder id in
            // place so the user sees the "waiting for first
            // token" state until the stream times out.
          }
        })
        .catch((e) => {
          resetAnalysisStream(newId);
          setStreamId("");
          setLoadError(String(e));
        });
    },
    [docs, docId, selection, source, sinceMs, parentAnalysisId],
  );

  // Per-card Analyze entry point: when the modal opens with
  // preselectRefs, tick the matched docs and kick off the
  // analyzer once they've loaded. Guarded by didAutoRunRef so a
  // later refresh doesn't re-fire the analyzer — the user can
  // re-trigger explicitly via the Analyze selected button if
  // they want a re-run. Placed after runAnalysis so the closure
  // it captures is the current one.
  useEffect(() => {
    if (didAutoRunRef.current) return;
    if (!preselectRefs || preselectRefs.length === 0) return;
    if (docs.length === 0) return;

    // Match preselectRefs against loaded docs by sha first, url
    // second. Silently ignore refs that don't resolve to a loaded
    // doc — they were probably for a doc outside the current
    // time window and we'd rather open a half-functional modal
    // than error the user out.
    const wanted = new Set(preselectRefs);
    const matched = new Set<string>();
    const resolvedRefs: string[] = [];
    docs.forEach((d) => {
      if (d.sha && wanted.has(d.sha)) {
        matched.add(docId(d));
        resolvedRefs.push(d.sha);
      } else if (d.url && wanted.has(d.url)) {
        matched.add(docId(d));
        resolvedRefs.push(d.url);
      }
    });
    if (resolvedRefs.length === 0) return;

    didAutoRunRef.current = true;
    // Visually tick the matched docs so the list pane reflects
    // what the analyzer is working over. The auto-run itself
    // passes explicit refs so it can't race the setSelection
    // commit — the checkboxes are purely cosmetic here.
    setSelection(matched);
    runAnalysis("selected", prompt, resolvedRefs);
  }, [docs, preselectRefs, docId, prompt, runAnalysis]);

  const runFollowup = useCallback(() => {
    if (!followupPrompt.trim() || stream.status !== "done") return;
    // A refinement uses the same scope as the parent's last run:
    // if the parent was over a selection, the refinement stays on
    // that selection; otherwise it's full-scope. We don't try to
    // carry over the parent's exact ref list — the parent's
    // analysis is already the "context", the refinement just asks
    // a follow-up against the same slice.
    const scope = selection.size > 0 ? "selected" : "all";
    // Stash the completed stream id as the parent of the next
    // refinement so the backend can thread the chain.
    setParentAnalysisId((prev) => prev || streamId);
    runAnalysis(scope, followupPrompt);
  }, [followupPrompt, stream.status, selection.size, runAnalysis, streamId]);

  const cancel = useCallback(() => {
    if (streamId) {
      CancelActivityAnalysis(streamId);
    }
  }, [streamId]);

  // ── Render ─────────────────────────────────────────────────────────
  const selectionCount = selection.size;
  const canAnalyzeSelected = selectionCount > 0 && docs.length > 0;
  const canAnalyzeAll = docs.length > 0;
  const isStreaming = stream.status === "streaming";
  const hasResult = stream.status === "done" || stream.status === "error" || stream.text.length > 0;

  return (
    <div
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 9000,
        background: "rgba(0,0,0,0.6)",
        backdropFilter: "blur(4px)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <div
        style={{
          width: "min(1200px, calc(100% - 48px))",
          height: "min(820px, calc(100vh - 48px))",
          background: "var(--bg-dark)",
          border: "1px solid var(--border)",
          borderRadius: 12,
          boxShadow: "0 20px 60px rgba(0,0,0,0.6)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        {/* ── Header ───────────────────────────────────────────────── */}
        <div
          style={{
            padding: "12px 18px",
            borderBottom: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            gap: 12,
          }}
        >
          <Sparkles size={14} style={{ color: "var(--cyan)" }} />
          <span
            style={{
              flex: 1,
              fontSize: 13,
              fontWeight: 600,
              color: "var(--fg)",
            }}
          >
            indexed docs · <span style={{ color: "var(--cyan)" }}>{source}</span>
            {sinceMs ? (
              <span style={{ fontSize: 10, color: "var(--fg-dim)", marginLeft: 8 }}>
                since {formatTime12(sinceMs)}
              </span>
            ) : null}
          </span>
          <button
            type="button"
            onClick={loadDocs}
            title="Reload doc list"
            style={ghostButton}
          >
            <RefreshCw size={13} />
          </button>
          <button type="button" onClick={onClose} title="Close (Esc)" style={ghostButton}>
            <X size={16} />
          </button>
        </div>

        {/* ── Body (two-pane) ──────────────────────────────────────── */}
        <div style={{ flex: 1, display: "flex", minHeight: 0 }}>
          {/* ── Left pane — doc list ─────────────────────────────── */}
          <div
            style={{
              width: 480,
              flexShrink: 0,
              borderRight: "1px solid var(--border)",
              display: "flex",
              flexDirection: "column",
              minHeight: 0,
            }}
          >
            {/* Selection header */}
            <div
              style={{
                padding: "8px 14px",
                borderBottom: "1px solid var(--border)",
                display: "flex",
                alignItems: "center",
                gap: 8,
                fontSize: 11,
                color: "var(--fg-dim)",
              }}
            >
              <span style={{ flex: 1 }}>
                {loading
                  ? "loading…"
                  : docs.length === 0
                    ? "no documents"
                    : `${docs.length} doc${docs.length === 1 ? "" : "s"} · ${selectionCount} selected`}
              </span>
              <button type="button" onClick={selectAll} style={textButton}>
                all
              </button>
              <button type="button" onClick={clearSelection} style={textButton}>
                none
              </button>
            </div>

            {/* Doc list */}
            <div style={{ flex: 1, overflowY: "auto" }}>
              {loadError && (
                <div
                  style={{
                    padding: "12px 14px",
                    color: "var(--red, #ff5555)",
                    fontSize: 11,
                    lineHeight: 1.5,
                  }}
                >
                  {loadError}
                </div>
              )}
              {!loading &&
                docs.length === 0 &&
                !loadError && (
                  <div
                    style={{
                      padding: "20px 14px",
                      color: "var(--fg-dim)",
                      fontSize: 11,
                      fontStyle: "italic",
                      lineHeight: 1.5,
                    }}
                  >
                    No documents in this window yet. When the collector indexes something new,
                    it'll appear here.
                  </div>
                )}
              {docs.map((d) => {
                const id = docId(d);
                const selected = selection.has(id);
                const expanded = expandedDoc === id;
                return (
                  <div
                    key={id}
                    style={{
                      padding: "10px 14px",
                      borderBottom: "1px solid var(--border)",
                      cursor: "pointer",
                      background: selected ? "rgba(139, 233, 253, 0.06)" : "transparent",
                    }}
                  >
                    <div
                      style={{ display: "flex", gap: 10, alignItems: "flex-start" }}
                      onClick={() => setExpandedDoc(expanded ? null : id)}
                    >
                      <input
                        type="checkbox"
                        checked={selected}
                        onChange={() => toggleSelect(d)}
                        onClick={(e) => e.stopPropagation()}
                        style={{ marginTop: 3, cursor: "pointer" }}
                      />
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div
                          style={{
                            color: "var(--fg)",
                            fontSize: 12,
                            lineHeight: 1.4,
                            fontWeight: 500,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {d.message || d.type || d.sha?.slice(0, 10) || "(untitled)"}
                        </div>
                        <div
                          style={{
                            marginTop: 3,
                            fontSize: 10,
                            color: "var(--fg-dim)",
                            fontFamily: "monospace",
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {[
                            d.type,
                            d.repo,
                            d.author && `@${d.author}`,
                            d.timestamp_ms && formatTime12(d.timestamp_ms),
                          ]
                            .filter(Boolean)
                            .join(" · ")}
                        </div>
                      </div>
                      {expanded ? (
                        <ChevronDown size={12} style={{ color: "var(--fg-dim)", marginTop: 4 }} />
                      ) : (
                        <ChevronRight size={12} style={{ color: "var(--fg-dim)", marginTop: 4 }} />
                      )}
                    </div>
                    {expanded && (
                      <div
                        style={{
                          marginTop: 8,
                          padding: "8px 10px",
                          background: "var(--bg)",
                          border: "1px solid var(--border)",
                          borderRadius: 4,
                          fontSize: 10,
                          lineHeight: 1.5,
                          color: "var(--fg-dim)",
                          fontFamily: "monospace",
                          whiteSpace: "pre-wrap",
                          wordBreak: "break-word",
                          maxHeight: 280,
                          overflowY: "auto",
                        }}
                      >
                        {JSON.stringify(d, null, 2)}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          {/* ── Right pane — analysis ─────────────────────────────── */}
          <div
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              minHeight: 0,
            }}
          >
            {/* Prompt + actions */}
            <div
              style={{
                padding: "12px 16px",
                borderBottom: "1px solid var(--border)",
                display: "flex",
                flexDirection: "column",
                gap: 8,
              }}
            >
              <textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="Optional: ask a specific question about these documents… (leave blank for a general read)"
                rows={2}
                disabled={isStreaming}
                style={{
                  width: "100%",
                  background: "var(--bg)",
                  border: "1px solid var(--border)",
                  borderRadius: 6,
                  padding: "8px 10px",
                  color: "var(--fg)",
                  fontSize: 12,
                  lineHeight: 1.5,
                  fontFamily: "inherit",
                  resize: "vertical",
                }}
              />
              <div style={{ display: "flex", gap: 8 }}>
                <button
                  type="button"
                  disabled={!canAnalyzeAll || isStreaming}
                  onClick={() => runAnalysis("all", prompt)}
                  style={primaryButton(!canAnalyzeAll || isStreaming)}
                >
                  <Play size={11} /> Analyze all ({docs.length})
                </button>
                <button
                  type="button"
                  disabled={!canAnalyzeSelected || isStreaming}
                  onClick={() => runAnalysis("selected", prompt)}
                  style={secondaryButton(!canAnalyzeSelected || isStreaming)}
                >
                  <Play size={11} /> Analyze selected ({selectionCount})
                </button>
                {isStreaming && (
                  <button type="button" onClick={cancel} style={dangerButton()}>
                    <Square size={11} /> Stop
                  </button>
                )}
              </div>
            </div>

            {/* Streaming output */}
            <div
              ref={analysisScrollRef}
              style={{
                flex: 1,
                overflowY: "auto",
                padding: "14px 18px",
                fontSize: 12,
                color: "var(--fg)",
                lineHeight: 1.6,
              }}
            >
              {!hasResult && !isStreaming && (
                <div
                  style={{
                    color: "var(--fg-dim)",
                    fontSize: 11,
                    fontStyle: "italic",
                    lineHeight: 1.6,
                  }}
                >
                  No analysis yet. Pick "Analyze all" or select a few docs and hit
                  "Analyze selected" — the model runs locally via opencode + qwen2.5:7b
                  and streams its findings here.
                </div>
              )}
              {isStreaming && stream.text.length === 0 && (
                <div
                  style={{
                    color: "var(--fg-dim)",
                    fontSize: 11,
                    fontStyle: "italic",
                  }}
                >
                  Waiting for first token…
                </div>
              )}
              {stream.text.length > 0 && (
                <div className="activity-analysis-markdown">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{stream.text}</ReactMarkdown>
                </div>
              )}
              {stream.status === "error" && (
                <div
                  style={{
                    marginTop: 12,
                    padding: "8px 10px",
                    background: "rgba(255, 85, 85, 0.08)",
                    border: "1px solid var(--red, #ff5555)",
                    borderRadius: 4,
                    color: "var(--red, #ff5555)",
                    fontSize: 11,
                    lineHeight: 1.5,
                  }}
                >
                  analysis failed: {stream.error}
                </div>
              )}
              {stream.status === "done" && stream.startedAt > 0 && (
                <div
                  style={{
                    marginTop: 10,
                    fontSize: 9,
                    color: "var(--fg-dim)",
                    fontFamily: "monospace",
                  }}
                >
                  took {((stream.endedAt - stream.startedAt) / 1000).toFixed(1)}s
                </div>
              )}
            </div>

            {/* Refinement footer — only visible after a completed run */}
            {stream.status === "done" && (
              <div
                style={{
                  padding: "10px 16px",
                  borderTop: "1px solid var(--border)",
                  display: "flex",
                  gap: 8,
                  alignItems: "center",
                }}
              >
                <input
                  type="text"
                  value={followupPrompt}
                  onChange={(e) => setFollowupPrompt(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      runFollowup();
                    }
                  }}
                  placeholder="Ask a follow-up (e.g. 'what's the first thing I should fix?')"
                  style={{
                    flex: 1,
                    background: "var(--bg)",
                    border: "1px solid var(--border)",
                    borderRadius: 6,
                    padding: "8px 10px",
                    color: "var(--fg)",
                    fontSize: 12,
                    fontFamily: "inherit",
                  }}
                />
                <button
                  type="button"
                  disabled={!followupPrompt.trim()}
                  onClick={runFollowup}
                  style={primaryButton(!followupPrompt.trim())}
                >
                  Refine
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// ── Button styles ─────────────────────────────────────────────────────
const ghostButton = {
  background: "none",
  border: "none",
  color: "var(--fg-dim)",
  cursor: "pointer",
  padding: 4,
  display: "flex",
} as const;

const textButton = {
  background: "none",
  border: "1px solid var(--border)",
  color: "var(--fg-dim)",
  cursor: "pointer",
  padding: "2px 8px",
  fontSize: 10,
  borderRadius: 4,
  textTransform: "uppercase" as const,
  letterSpacing: "0.06em",
};

function primaryButton(disabled: boolean) {
  return {
    background: disabled ? "var(--bg)" : "var(--cyan)",
    border: "1px solid var(--cyan)",
    color: disabled ? "var(--fg-dim)" : "var(--bg-dark)",
    cursor: disabled ? "not-allowed" : "pointer",
    padding: "6px 12px",
    fontSize: 11,
    fontWeight: 600,
    borderRadius: 6,
    display: "flex",
    alignItems: "center",
    gap: 6,
    opacity: disabled ? 0.5 : 1,
  } as const;
}

function secondaryButton(disabled: boolean) {
  return {
    background: "transparent",
    border: "1px solid var(--border)",
    color: disabled ? "var(--fg-dim)" : "var(--fg)",
    cursor: disabled ? "not-allowed" : "pointer",
    padding: "6px 12px",
    fontSize: 11,
    fontWeight: 500,
    borderRadius: 6,
    display: "flex",
    alignItems: "center",
    gap: 6,
    opacity: disabled ? 0.5 : 1,
  } as const;
}

function dangerButton() {
  return {
    background: "transparent",
    border: "1px solid var(--red, #ff5555)",
    color: "var(--red, #ff5555)",
    cursor: "pointer",
    padding: "6px 12px",
    fontSize: 11,
    fontWeight: 500,
    borderRadius: 6,
    display: "flex",
    alignItems: "center",
    gap: 6,
  } as const;
}

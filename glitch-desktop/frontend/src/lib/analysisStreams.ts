/**
 * analysisStreams — standalone stream buffer store for ad-hoc
 * activity analyses. The backend's AnalyzeActivityChunks binding
 * streams tokens over the brain:analysis:stream Wails event keyed
 * by a streamId. Each stream belongs to one open IndexedDocsModal
 * surface, and only that surface needs to re-render when tokens
 * land — going through the main chat reducer on every token would
 * re-render the entire App on every character, which is a lot of
 * wasted work during a 500-token analysis.
 *
 * Instead we keep a small singleton map of streamId → buffer +
 * status and expose a `useAnalysisStream(streamId)` hook built on
 * React's useSyncExternalStore. Only components that subscribe to
 * a specific streamId re-render when that stream updates.
 *
 * Lifecycle: the modal creates a streamId when it invokes
 * AnalyzeActivityChunks, subscribes via useAnalysisStream, and
 * calls `resetAnalysisStream(id)` when the user closes the modal
 * (or starts a fresh refinement) to free memory. Streams not
 * explicitly reset stick around for the process lifetime, which
 * is fine for a desktop app's memory budget.
 */
import { useSyncExternalStore } from "react";
import type { AnalysisStreamEvent } from "./types";

export type AnalysisStreamStatus = "idle" | "streaming" | "done" | "error";

export interface AnalysisStreamState {
  /** Concatenated text content received so far (markdown). */
  text: string;
  /** Current lifecycle state of the stream. */
  status: AnalysisStreamStatus;
  /** Human-readable error from the terminal "error" event. */
  error: string;
  /** Wall-clock time the stream started (ms epoch). Used by the
   *  modal to show "took 12.3s" after completion. */
  startedAt: number;
  /** Wall-clock time the stream terminated, or 0 while running. */
  endedAt: number;
}

function blankState(): AnalysisStreamState {
  return { text: "", status: "idle", error: "", startedAt: 0, endedAt: 0 };
}

// Singleton state + listener map. Both are module-private; callers
// interact via the exported functions. Using a plain object (rather
// than Map) keeps equality comparisons cheap in React — we create
// a fresh state reference on every mutation, which is what
// useSyncExternalStore needs to detect changes.
const streams: Record<string, AnalysisStreamState> = {};
const listeners: Record<string, Set<() => void>> = {};

function notify(streamId: string): void {
  const set = listeners[streamId];
  if (!set) return;
  set.forEach((l) => l());
}

/**
 * startAnalysisStream marks a stream as "streaming" and stamps
 * its start time. Called by the modal immediately before the
 * AnalyzeActivityChunks invocation so useAnalysisStream can
 * render a "waiting for first token" spinner.
 */
export function startAnalysisStream(streamId: string): void {
  streams[streamId] = {
    ...blankState(),
    status: "streaming",
    startedAt: Date.now(),
  };
  notify(streamId);
}

/**
 * appendAnalysisStream is the single entry point the App's
 * brain:analysis:stream handler calls on every incoming event.
 * Token events append to the buffer; done/error events terminate
 * the stream. A done/error on a stream we haven't seen before
 * implicitly creates it so the modal can still render the final
 * state — in practice this only happens if the modal lost the
 * startAnalysisStream call (e.g. on a fast error).
 */
export function appendAnalysisStream(
  streamId: string,
  ev: AnalysisStreamEvent,
): void {
  const prev = streams[streamId] ?? blankState();
  let next: AnalysisStreamState;
  switch (ev.kind) {
    case "token":
      next = {
        ...prev,
        status: prev.status === "idle" ? "streaming" : prev.status,
        text: prev.text + (ev.data ?? ""),
        startedAt: prev.startedAt || Date.now(),
      };
      break;
    case "done":
      next = {
        ...prev,
        status: "done",
        endedAt: Date.now(),
      };
      break;
    case "error":
      next = {
        ...prev,
        status: "error",
        error: ev.error ?? "unknown error",
        endedAt: Date.now(),
      };
      break;
    default:
      next = prev;
  }
  streams[streamId] = next;
  notify(streamId);
}

/**
 * resetAnalysisStream drops a stream's buffer and status. Called
 * when the modal closes or when the user starts a fresh analysis
 * in the same surface. Safe to call on unknown ids.
 */
export function resetAnalysisStream(streamId: string): void {
  if (streams[streamId]) {
    delete streams[streamId];
    notify(streamId);
  }
}

/**
 * getAnalysisStream returns the current state snapshot for a
 * stream id. Used by useSyncExternalStore's getSnapshot. Returning
 * the blank constant for unknown ids keeps the reference stable
 * across renders (React's external-store contract requires this).
 */
const BLANK: AnalysisStreamState = Object.freeze(blankState());
export function getAnalysisStream(streamId: string): AnalysisStreamState {
  return streams[streamId] ?? BLANK;
}

/**
 * useAnalysisStream subscribes the calling component to a single
 * stream id and returns its current state. The component re-renders
 * only when that specific stream changes — unrelated streams and
 * unrelated app state do not trigger re-renders.
 */
export function useAnalysisStream(streamId: string): AnalysisStreamState {
  const subscribe = (cb: () => void) => {
    if (!streamId) return () => {};
    let set = listeners[streamId];
    if (!set) {
      set = new Set();
      listeners[streamId] = set;
    }
    set.add(cb);
    return () => {
      set.delete(cb);
      if (set.size === 0) delete listeners[streamId];
    };
  };
  const getSnapshot = () => (streamId ? getAnalysisStream(streamId) : BLANK);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

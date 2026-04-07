// PausePanel renders the inline pause affordance that sits between the
// message list and the chat input whenever a step-through session has a
// step waiting on a user decision. It is the UI half of the
// "step-through isn't a mode, it's a property of the run" rule: for
// multi-step chain runs we pause between steps and surface the captured
// output here so the user can accept, hand-edit, retry, or save-as
// without leaving the chat surface.
//
// Design notes:
// - The draft textarea is owned LOCALLY by this component (not lifted
//   to the parent). For a 60 KB monospace textarea, pushing every
//   keystroke to App-level state re-rendered the chat surface and
//   caused visible scroll lag — the user flagged this on a grep-dump
//   screenshot. Local state keeps keystrokes inside PausePanel.
//   When the parent sees a new pause, originalOutput changes, and a
//   useEffect resyncs the local draft.
// - Edit & continue reads the local draft via a ref-stable callback
//   so the parent can resume the session with the current value.
// - Save-as uses a one-line inline name input rather than a modal —
//   the goal is to make "save this exploration" feel as cheap as
//   accepting a step. A modal would make it feel like a commitment.
// - Edit vs Retry are two distinct loops. Edit accepts the run's
//   output but overrides the captured value (cheap, no re-run). Retry
//   throws out the current run entirely and replays the chain from
//   the top (expensive, honest).
import { useState, useEffect, useRef, type CSSProperties } from "react";

interface Props {
  stepId: string;
  /** 0-based index of the paused step within the chain. */
  stepIndex: number;
  /** Total number of steps in the chain. */
  stepTotal: number;
  originalOutput: string;
  onAccept: () => void;
  onEditAndContinue: (editedValue: string) => void;
  onRetry: () => void;
  onAbort: () => void;
  onSaveAs: (name: string) => void;
}

export function PausePanel({
  stepId,
  stepIndex,
  stepTotal,
  originalOutput,
  onAccept,
  onEditAndContinue,
  onRetry,
  onAbort,
  onSaveAs,
}: Props) {
  // Whether this is the last step. The pause is post-step, so when
  // index == total-1 the runner has nothing left to do once we accept;
  // the button should say "Finish" so the user doesn't expect another
  // pause to follow. Earlier pauses say "Continue" because clicking
  // them visibly advances the chain to the next step.
  const isLastStep = stepTotal > 0 && stepIndex >= stepTotal - 1;
  const acceptLabel = isLastStep ? "Finish" : "Continue →";
  const editLabel = isLastStep ? "Use edit & finish" : "Use edit & continue";
  const [saveName, setSaveName] = useState("");
  const [showSave, setShowSave] = useState(false);
  // Local draft state. Initialized from originalOutput; resynced via the
  // effect below when the parent surfaces a new paused step. The dirty
  // bit is derived from the draft so the Accept / Edit & continue
  // disabled state stays in sync without an extra setState round-trip.
  const [draft, setDraft] = useState(originalOutput);
  const draftRef = useRef(draft);
  draftRef.current = draft;

  useEffect(() => {
    setDraft(originalOutput);
  }, [originalOutput]);

  const dirty = draft !== originalOutput;

  return (
    <div style={wrap}>
      <div style={header}>
        <span style={label}>
          <span style={dot} />
          paused · step {stepIndex + 1} of {stepTotal || "?"}
          <span style={stepIdBadge}>{stepId}</span>
        </span>
        <span style={hint}>
          {dirty
            ? "edited — use edit & continue to commit"
            : isLastStep
            ? "this is the last step — Finish to wrap up"
            : `Continue to run step ${stepIndex + 2} of ${stepTotal}`}
        </span>
      </div>
      <textarea
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        spellCheck={false}
        style={textareaStyle}
        placeholder="(no output)"
      />
      <div style={actions}>
        <button
          onClick={onAccept}
          style={primaryBtn}
          disabled={dirty}
          title={
            dirty
              ? "Use edit & continue to commit your edits"
              : isLastStep
              ? "Finish the run"
              : `Run the next step (${stepIndex + 2} of ${stepTotal})`
          }
        >
          {acceptLabel}
        </button>
        <button
          onClick={() => onEditAndContinue(draftRef.current)}
          style={dirty ? primaryBtn : secondaryBtn}
          disabled={!dirty}
          title={dirty ? "Use the edited output and continue" : "No edits to apply"}
        >
          {editLabel}
        </button>
        <button onClick={onRetry} style={secondaryBtn} title="Abort and replay the chain from the start">
          Retry
        </button>
        <button onClick={onAbort} style={dangerBtn} title="Abort the session">
          Abort
        </button>
        <div style={{ flex: 1 }} />
        {showSave ? (
          <>
            <input
              autoFocus
              value={saveName}
              onChange={(e) => setSaveName(e.target.value)}
              placeholder="workflow name"
              style={nameInput}
              onKeyDown={(e) => {
                if (e.key === "Enter" && saveName.trim()) {
                  onSaveAs(saveName.trim());
                  setShowSave(false);
                  setSaveName("");
                } else if (e.key === "Escape") {
                  setShowSave(false);
                  setSaveName("");
                }
              }}
            />
            <button
              onClick={() => {
                if (saveName.trim()) {
                  onSaveAs(saveName.trim());
                  setShowSave(false);
                  setSaveName("");
                }
              }}
              style={secondaryBtn}
              disabled={!saveName.trim()}
            >
              Save
            </button>
          </>
        ) : (
          <button
            onClick={() => setShowSave(true)}
            style={secondaryBtn}
            title="Save the current chain as a .workflow.yaml"
          >
            Save as…
          </button>
        )}
      </div>
    </div>
  );
}

const wrap: CSSProperties = {
  borderTop: "1px solid var(--border)",
  background: "var(--bg-alt, rgba(255,255,255,0.03))",
  padding: "10px 14px",
  display: "flex",
  flexDirection: "column",
  gap: 8,
  fontSize: 12,
};

const header: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: 12,
};

const label: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 8,
  color: "var(--fg)",
  fontWeight: 600,
};

const stepIdBadge: CSSProperties = {
  fontWeight: 400,
  fontSize: 10,
  color: "var(--muted)",
  background: "var(--bg)",
  border: "1px solid var(--border)",
  borderRadius: 3,
  padding: "1px 6px",
  fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, monospace)",
};

const dot: CSSProperties = {
  width: 8,
  height: 8,
  borderRadius: "50%",
  background: "var(--accent, #bd93f9)",
  boxShadow: "0 0 8px var(--accent, #bd93f9)",
};

const hint: CSSProperties = {
  color: "var(--muted)",
  fontSize: 11,
};

const textareaStyle: CSSProperties = {
  width: "100%",
  // Roomy by default — the captured step output is usually a full LLM
  // response, so a 3-line textarea hides everything. The user pulled us
  // up on this; ~18 lines is the sweet spot before the pause panel
  // starts crowding the chat surface.
  minHeight: 280,
  maxHeight: "60vh",
  resize: "vertical",
  background: "var(--bg)",
  color: "var(--fg)",
  border: "1px solid var(--border)",
  borderRadius: 4,
  padding: "8px 10px",
  fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, monospace)",
  fontSize: 12,
  lineHeight: 1.5,
  outline: "none",
  // Force horizontal scrolling instead of soft-wrapping. Soft-wrap on a
  // 60 KB monospace textarea makes Webkit recompute line metrics on
  // every keystroke and on every scroll frame; pre + horizontal scroll
  // skips that work entirely. This was the visible scroll lag the user
  // reported on the grep-dump screenshot.
  whiteSpace: "pre",
  overflowX: "auto",
  overflowY: "auto",
};

const actions: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 6,
};

const baseBtn: CSSProperties = {
  border: "1px solid var(--border)",
  borderRadius: 4,
  padding: "4px 12px",
  cursor: "pointer",
  fontSize: 12,
  background: "transparent",
  color: "var(--fg)",
};

const primaryBtn: CSSProperties = {
  ...baseBtn,
  background: "var(--accent, #bd93f9)",
  color: "var(--bg)",
  borderColor: "transparent",
  fontWeight: 600,
};

const secondaryBtn: CSSProperties = {
  ...baseBtn,
  color: "var(--muted)",
};

const dangerBtn: CSSProperties = {
  ...baseBtn,
  color: "var(--error, #ff5555)",
  borderColor: "var(--error, #ff5555)",
};

const nameInput: CSSProperties = {
  background: "var(--bg)",
  color: "var(--fg)",
  border: "1px solid var(--border)",
  borderRadius: 4,
  padding: "4px 8px",
  fontSize: 12,
  minWidth: 180,
  outline: "none",
};

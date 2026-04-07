/**
 * EditorPopup — the canonical editor surface for prompts and
 * workflows in gl1tch.
 *
 * Two halves stacked vertically:
 *   1. CodeMirror editor showing the draft body (markdown for prompts,
 *      YAML for workflows). Edits flow into local state and a save
 *      action persists them via PromoteDraft.
 *   2. Chat strip at the bottom — the user types an instruction,
 *      gl1tch refines the body via RefineDraft, and the result streams
 *      back into the editor in real time. The provider picker on the
 *      strip lets the user swap to a "power answer" model for one
 *      turn; the picker snaps back to observer mode after the turn
 *      completes so the next refinement uses the workspace default.
 *
 * Streams are keyed by draft id, so two popups can be open and
 * refining different drafts simultaneously without their tokens
 * crossing wires. Subscriptions filter on the open draft's id.
 */
import { useEffect, useMemo, useRef, useState } from "react";
import { X, Save, Play, RotateCcw, FileText, Workflow, Sparkles, ArrowUp, Square, Bot, Lock, Copy, Settings } from "lucide-react";
import CodeMirror from "@uiw/react-codemirror";
import { markdown } from "@codemirror/lang-markdown";
import { yaml } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";

import { ProviderPicker } from "./ProviderPicker";
import type { PickerProviderOption } from "./ProviderPicker";
import { useToast } from "./Toast";
import { useWailsEvent } from "../lib/wails";
import {
  GetDraft,
  RefineDraft,
  StopDraftRefine,
  PromoteDraft,
  PromoteDraftAs,
  UpdateDraftBody,
  DeleteDraft,
  RunWorkflow,
} from "../../wailsjs/go/main/App";

export type EditorKind = "prompt" | "workflow" | "skill" | "agent" | "collectors";

interface DraftJSON {
  id: number;
  workspace_id: string;
  kind: string;
  title: string;
  body: string;
  turns: Array<{
    role: string;
    text: string;
    body?: string;
    provider?: string;
    model?: string;
    timestamp: number;
  }>;
  target_id?: number;
  target_path?: string;
  /** Set by the backend when target_path lives outside the workspace
   *  (i.e. ~/.claude/, ~/.stok/, etc.). The popup uses this to lock
   *  out the save button and force the user through "save as new". */
  read_only?: boolean;
  created_at: number;
  updated_at: number;
}

interface Props {
  /** Draft id to edit. The popup loads via GetDraft on mount. */
  draftId: number;
  /** Used for run-after-save (workflow only). */
  workspaceId: string;
  /** Provider/model picker state from the parent. The popup keeps its
   *  own per-turn override so the parent's picker isn't disturbed. */
  providers: PickerProviderOption[];
  observerDefaultProvider: string;
  observerDefaultModel: string;
  /** Pin a model as the observer default. Same shape as ChatInput's
   *  callback so the picker behaves identically across surfaces. */
  onSetObserverDefault: (providerId: string, modelId: string) => void;
  /** Called when the user closes the popup via Esc / X / save+run. */
  onClose: () => void;
  /** Called after a successful save so the parent can refresh the
   *  sidebar (workflows list, prompts list, etc.). */
  onSaved: () => void;
}

export function EditorPopup({
  draftId,
  workspaceId,
  providers,
  observerDefaultProvider,
  observerDefaultModel,
  onSetObserverDefault,
  onClose,
  onSaved,
}: Props) {
  const toast = useToast();

  const [draft, setDraft] = useState<DraftJSON | null>(null);
  const [liveBody, setLiveBody] = useState("");
  const [title, setTitle] = useState("");
  const [loading, setLoading] = useState(true);

  // Per-turn provider override. Empty string = use the observer
  // default. We snap this back to "" after every successful refine so
  // the user can spike a single turn at a "power model" without that
  // override silently sticking around for the rest of the session.
  const [turnProvider, setTurnProvider] = useState("");
  const [turnModel, setTurnModel] = useState("");

  const [chatInput, setChatInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  // Buffer of tokens received during the in-flight refine. Reset on
  // each send. We also push it directly into liveBody so the user
  // sees the editor update token-by-token.
  const streamBufferRef = useRef("");

  const [saving, setSaving] = useState(false);

  const kind: EditorKind = (draft?.kind as EditorKind) ?? "prompt";
  const isWorkflow = kind === "workflow";
  const isSkill = kind === "skill";
  const isAgent = kind === "agent";
  const isCollectors = kind === "collectors";
  // YAML for workflow + collectors; markdown for prompt/skill/agent.
  const isMarkdown = !isWorkflow && !isCollectors;
  const readOnly = !!draft?.read_only;
  const dirty = !!draft && liveBody !== draft.body;
  const titleDirty = !!draft && title !== draft.title;

  // ── Initial load ────────────────────────────────────────────────────
  useEffect(() => {
    let cancelled = false;
    GetDraft(draftId).then((json) => {
      if (cancelled) return;
      try {
        const d = JSON.parse(json) as DraftJSON;
        if (!d || !d.id) {
          toast.error("Couldn't load draft", { detail: "Draft not found" });
          onClose();
          return;
        }
        setDraft(d);
        setLiveBody(d.body ?? "");
        setTitle(d.title ?? "");
      } catch (e) {
        toast.error("Couldn't load draft", { detail: String(e) });
        onClose();
      } finally {
        setLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
    // Intentionally omit toast/onClose — they're stable refs from
    // their providers and re-running this effect would re-fetch the
    // draft on every parent rerender.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draftId]);

  // ── Stream subscriptions ────────────────────────────────────────────
  // Each handler filters on the popup's draft id so multiple popups
  // don't cross-contaminate. The chunks coming back are NOT
  // assistant chatter — they're tokens of the new body itself (the
  // system prompt enforces this), so we display them as the body
  // being progressively rewritten.
  useWailsEvent("draft:chunk", (data: unknown) => {
    const d = data as { draft_id?: number; text?: string };
    if (d?.draft_id !== draftId || !d.text) return;
    streamBufferRef.current += d.text;
    setLiveBody(streamBufferRef.current);
  });

  useWailsEvent("draft:done", (data: unknown) => {
    const d = data as { draft_id?: number };
    if (d?.draft_id !== draftId) return;
    setStreaming(false);
    // Re-fetch the draft so we get the canonical persisted body and
    // updated turns from SQLite. The streamed tokens we already have
    // should match, but the persisted version is the source of truth
    // and includes any trim/normalization the backend applied.
    GetDraft(draftId).then((json) => {
      try {
        const next = JSON.parse(json) as DraftJSON;
        setDraft(next);
        setLiveBody(next.body ?? "");
      } catch {}
    });
    // Snap the per-turn provider override back to observer mode so
    // the next refinement uses the workspace default again. This is
    // the "super response without sticking" behavior the user
    // explicitly asked for.
    setTurnProvider("");
    setTurnModel("");
  });

  useWailsEvent("draft:error", (data: unknown) => {
    const d = data as { draft_id?: number; message?: string };
    if (d?.draft_id !== draftId) return;
    setStreaming(false);
    toast.error("Refine failed", {
      detail: d.message ?? "Unknown error",
      actions: [
        {
          label: "Retry",
          onClick: () => sendRefinement(),
          dismissOnClick: true,
        },
      ],
    });
  });

  // ── Keyboard shortcuts ──────────────────────────────────────────────
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        if (streaming) {
          // First Esc cancels the in-flight refine; second Esc closes.
          StopDraftRefine(draftId);
          return;
        }
        onClose();
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "s") {
        e.preventDefault();
        save();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streaming, draftId, liveBody, title, dirty, titleDirty]);

  // ── Actions ─────────────────────────────────────────────────────────

  function sendRefinement() {
    const text = chatInput.trim();
    if (!text || streaming) return;
    streamBufferRef.current = "";
    setStreaming(true);
    setChatInput("");
    RefineDraft(draftId, text, turnProvider, turnModel);
  }

  function stopRefinement() {
    StopDraftRefine(draftId);
    // streaming flag will get cleared by the draft:error handler when
    // the canceled run reports back. Optimistically clear it here too
    // so the UI feels responsive.
    setStreaming(false);
  }

  function revert() {
    if (!draft) return;
    setLiveBody(draft.body ?? "");
    setTitle(draft.title ?? "");
  }

  async function save(): Promise<boolean> {
    if (!draft || saving) return false;
    if (readOnly) {
      // Block the save path entirely on read-only drafts. The "save
      // as new" button is the user's only way to commit changes to a
      // global entity — it forks into a workspace copy.
      toast.push({
        title: "This entry is read-only",
        detail: "Use “save as new” to fork it into a workspace copy.",
        severity: "warn",
      });
      return false;
    }
    if (!title.trim()) {
      toast.push({ title: "Give your draft a title", severity: "warn" });
      return false;
    }
    setSaving(true);
    try {
      // First, persist any local CodeMirror edits to the draft row so
      // PromoteDraft sees the current title/body — without this,
      // manual edits would be silently dropped because PromoteDraft
      // reads from SQLite, not from popup state.
      const updateErr = await UpdateDraftBody(draftId, title, liveBody);
      if (updateErr) {
        toast.error("Couldn't save", {
          detail: updateErr,
          actions: [{ label: "Retry", onClick: () => save() }],
        });
        return false;
      }

      const result = await PromoteDraft(draftId, false);
      const parsed = JSON.parse(result);
      if (parsed?.error) {
        toast.error("Couldn't save", {
          detail: parsed.error,
          actions: [{ label: "Retry", onClick: () => save() }],
        });
        return false;
      }
      toast.success(
        isWorkflow ? `Saved ${title}.workflow.yaml` : `Saved prompt "${title}"`,
        { detail: parsed.target_path || undefined },
      );
      // Refresh local state from the canonical row so target_path /
      // target_id reflects whatever the promote backfilled.
      const fresh = await GetDraft(draftId);
      try {
        const next = JSON.parse(fresh) as DraftJSON;
        setDraft(next);
        setLiveBody(next.body ?? "");
        setTitle(next.title ?? "");
      } catch {}
      onSaved();
      return true;
    } finally {
      setSaving(false);
    }
  }

  async function saveAndRun() {
    const ok = await save();
    if (!ok || !isWorkflow) return;
    // Re-read the draft to get the freshly assigned target_path.
    try {
      const json = await GetDraft(draftId);
      const next = JSON.parse(json) as DraftJSON;
      if (next?.target_path) {
        RunWorkflow(next.target_path, "", workspaceId);
        onClose();
      }
    } catch {}
  }

  // saveAsNew is the "fork this into a workspace copy" path. Used
  // primarily for read-only global entities (skills/agents from
  // ~/.claude) but also works as a convenience "save under a
  // different name" for any draft. Prompts the user for the new
  // name first; if they cancel, nothing happens.
  async function saveAsNew() {
    if (!draft || saving) return;
    // Persist any in-flight CodeMirror edits before forking so the
    // new copy reflects the current editor state, not the stale
    // SQLite snapshot.
    const updateErr = await UpdateDraftBody(draftId, title, liveBody);
    if (updateErr) {
      toast.error("Couldn't fork draft", { detail: updateErr });
      return;
    }
    // Suggest "<title>-copy" as the default new name so the user
    // doesn't accidentally promote with the original name (which
    // would either collide or write back to the same place).
    const suggested = title.trim() ? `${title.trim()}-copy` : "";
    const next = window.prompt("Save as new — name?", suggested);
    if (!next || !next.trim()) return;

    setSaving(true);
    try {
      const result = await PromoteDraftAs(draftId, next.trim());
      const parsed = JSON.parse(result);
      if (parsed?.error) {
        toast.error("Couldn't save as new", {
          detail: parsed.error,
          actions: [{ label: "Retry", onClick: () => saveAsNew() }],
        });
        return;
      }
      toast.success(`Forked into workspace as "${next.trim()}"`, {
        detail: parsed.target_path || undefined,
      });
      // Reload from the freshly promoted draft so the popup is now
      // pointed at the new workspace target. Subsequent saves will
      // overwrite the fork rather than re-prompting.
      const fresh = await GetDraft(draftId);
      try {
        const d = JSON.parse(fresh) as DraftJSON;
        setDraft(d);
        setLiveBody(d.body ?? "");
        setTitle(d.title ?? "");
      } catch {}
      onSaved();
    } finally {
      setSaving(false);
    }
  }

  async function discardAndClose() {
    // For brand-new drafts (no saved target) we delete the draft row
    // entirely so the next "new" click starts fresh instead of
    // resurrecting the abandoned body.
    if (draft && !draft.target_id && !draft.target_path) {
      await DeleteDraft(draftId);
    }
    onClose();
  }

  // ── Editor language extension ───────────────────────────────────────
  // CodeMirror extensions are an array; we memoize so the editor
  // doesn't churn its internal state on every parent rerender.
  const extensions = useMemo(() => {
    return isMarkdown ? [markdown()] : [yaml()];
  }, [isMarkdown]);

  // Per-kind icon + accent color so the header reads at a glance which
  // entity type the popup is editing. Skills get green, agents get
  // purple — same convention the sidebar uses. Collectors get the
  // settings cog in cyan to match the brain popover that opens them.
  let KindIcon = FileText;
  let kindColor = "var(--orange)";
  if (isWorkflow) {
    KindIcon = Workflow;
    kindColor = "var(--cyan)";
  } else if (isSkill) {
    KindIcon = FileText;
    kindColor = "var(--green)";
  } else if (isAgent) {
    KindIcon = Bot;
    kindColor = "var(--purple)";
  } else if (isCollectors) {
    KindIcon = Settings;
    kindColor = "var(--cyan)";
  }

  return (
    <div
      onClick={(e) => {
        if (e.target === e.currentTarget) discardAndClose();
      }}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        backdropFilter: "blur(4px)",
        zIndex: 9000,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 24,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          width: "min(960px, 100%)",
          height: "min(720px, calc(100vh - 48px))",
          background: "var(--bg-dark)",
          border: "1px solid var(--border)",
          borderRadius: 12,
          boxShadow: "0 20px 60px rgba(0,0,0,0.6)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        {/* ── Header ─────────────────────────────────────────────── */}
        <div
          style={{
            padding: "12px 18px",
            borderBottom: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            gap: 12,
          }}
        >
          <KindIcon size={14} style={{ color: kindColor }} />
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={isWorkflow ? "workflow name" : "prompt title"}
            spellCheck={false}
            style={{
              flex: 1,
              background: "transparent",
              border: "none",
              outline: "none",
              color: "var(--fg)",
              fontSize: 14,
              fontWeight: 600,
              fontFamily: "inherit",
              padding: "4px 0",
            }}
          />
          <span
            style={{
              fontSize: 9,
              color: readOnly ? "var(--yellow)" : "var(--fg-dim)",
              textTransform: "uppercase",
              letterSpacing: "0.06em",
              border: "1px solid " + (readOnly ? "var(--yellow)" : "var(--border)"),
              padding: "2px 6px",
              borderRadius: 4,
              display: "flex",
              alignItems: "center",
              gap: 4,
            }}
            title={
              readOnly
                ? "Read-only — global entry. Use “save as new” to fork into this workspace."
                : "Workspace-scoped — saved under this workspace's primary directory"
            }
          >
            {readOnly && <Lock size={9} />}
            {readOnly ? "global · read-only" : "workspace"}
          </span>
          <button
            onClick={discardAndClose}
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

        {draft?.target_path && (
          <div
            style={{
              padding: "4px 18px 8px",
              fontSize: 10,
              color: "var(--fg-dim)",
              fontFamily: "monospace",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
            title={draft.target_path}
          >
            {draft.target_path}
          </div>
        )}

        {/* ── Editor ─────────────────────────────────────────────── */}
        <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
          {loading ? (
            <div
              style={{
                flex: 1,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                color: "var(--fg-dim)",
                fontSize: 12,
              }}
            >
              loading draft…
            </div>
          ) : (
            <div
              style={{
                flex: 1,
                minHeight: 0,
                overflow: "auto",
                background: "var(--bg)",
                borderTop: "1px solid var(--border)",
                borderBottom: "1px solid var(--border)",
              }}
            >
              <CodeMirror
                value={liveBody}
                onChange={(v) => setLiveBody(v)}
                theme={oneDark}
                extensions={extensions}
                editable={!readOnly}
                height="100%"
                style={{
                  fontSize: 13,
                  fontFamily:
                    "Berkeley Mono, JetBrains Mono, Fira Code, SF Mono, monospace",
                }}
                basicSetup={{
                  lineNumbers: true,
                  highlightActiveLine: true,
                  foldGutter: false,
                  highlightActiveLineGutter: true,
                }}
              />
            </div>
          )}
        </div>

        {/* ── Chat strip ─────────────────────────────────────────── */}
        <div
          style={{
            padding: "10px 18px",
            background: "var(--bg-dark)",
            display: "flex",
            alignItems: "flex-end",
            gap: 8,
          }}
        >
          <Sparkles size={12} style={{ color: "var(--purple)", marginBottom: 8 }} />
          <textarea
            value={chatInput}
            onChange={(e) => setChatInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                sendRefinement();
              }
            }}
            placeholder={
              streaming
                ? "gl1tch is refining…"
                : draft?.body
                ? "How should I refine this?"
                : "What should this prompt do?"
            }
            disabled={streaming}
            rows={1}
            style={{
              flex: 1,
              background: "var(--bg)",
              border: "1px solid var(--border)",
              borderRadius: 8,
              padding: "8px 10px",
              color: "var(--fg)",
              fontSize: 12,
              fontFamily: "inherit",
              resize: "none",
              outline: "none",
              minHeight: 32,
              maxHeight: 100,
            }}
          />
          <ProviderPicker
            providers={providers}
            selectedProvider={turnProvider}
            selectedModel={turnModel}
            observerDefaultProvider={observerDefaultProvider}
            observerDefaultModel={observerDefaultModel}
            onSelectProvider={(p, m) => {
              setTurnProvider(p);
              setTurnModel(m);
            }}
            onSetObserverDefault={onSetObserverDefault}
            align="right"
          />
          {streaming ? (
            <button
              onClick={stopRefinement}
              title="Stop refine (Esc)"
              style={{
                width: 32,
                height: 32,
                borderRadius: 8,
                background: "var(--red)",
                border: "none",
                color: "var(--bg-dark)",
                cursor: "pointer",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Square size={12} strokeWidth={3} fill="currentColor" />
            </button>
          ) : (
            <button
              onClick={sendRefinement}
              disabled={!chatInput.trim()}
              title="Refine (Enter)"
              style={{
                width: 32,
                height: 32,
                borderRadius: 8,
                background: chatInput.trim() ? "var(--purple)" : "var(--bg-surface)",
                border: "none",
                color: chatInput.trim() ? "var(--bg-dark)" : "var(--fg-dim)",
                cursor: chatInput.trim() ? "pointer" : "default",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <ArrowUp size={14} strokeWidth={2.5} />
            </button>
          )}
        </div>

        {/* ── Footer ─────────────────────────────────────────────── */}
        <div
          style={{
            padding: "10px 18px",
            borderTop: "1px solid var(--border)",
            background: "var(--bg-dark)",
            display: "flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          <div style={{ flex: 1, fontSize: 10, color: "var(--fg-dim)" }}>
            {dirty || titleDirty
              ? "unsaved changes · ⌘S to save · esc to close"
              : draft?.updated_at
              ? "saved"
              : ""}
          </div>
          {(dirty || titleDirty) && !readOnly && (
            <button
              onClick={revert}
              style={{
                background: "none",
                border: "1px solid var(--border)",
                color: "var(--fg-dim)",
                padding: "5px 10px",
                borderRadius: 6,
                cursor: "pointer",
                fontSize: 11,
                display: "flex",
                alignItems: "center",
                gap: 4,
              }}
              title="Discard local changes"
            >
              <RotateCcw size={10} />
              revert
            </button>
          )}
          {/* "Save as new" — always available; the canonical fork
              path for global read-only entries and a convenience
              under-a-different-name save for everything else. */}
          <button
            onClick={saveAsNew}
            disabled={saving}
            style={{
              background: "transparent",
              border: "1px solid var(--purple)",
              color: "var(--purple)",
              padding: "5px 12px",
              borderRadius: 6,
              cursor: saving ? "default" : "pointer",
              fontSize: 11,
              fontWeight: 600,
              opacity: saving ? 0.5 : 1,
              display: "flex",
              alignItems: "center",
              gap: 4,
            }}
            title="Fork this draft into a new workspace entry under a different name"
          >
            <Copy size={10} />
            save as new
          </button>
          {isWorkflow && !readOnly && (
            <button
              onClick={saveAndRun}
              disabled={saving || !title.trim()}
              style={{
                background: "transparent",
                border: "1px solid var(--green)",
                color: "var(--green)",
                padding: "5px 12px",
                borderRadius: 6,
                cursor: saving || !title.trim() ? "default" : "pointer",
                fontSize: 11,
                fontWeight: 600,
                opacity: saving || !title.trim() ? 0.5 : 1,
                display: "flex",
                alignItems: "center",
                gap: 4,
              }}
              title="Save and run this workflow now"
            >
              <Play size={10} />
              save &amp; run
            </button>
          )}
          {!readOnly && (
            <button
              onClick={save}
              disabled={saving || !title.trim()}
              style={{
                background: title.trim() ? "var(--cyan)" : "var(--bg-surface)",
                color: title.trim() ? "var(--bg-dark)" : "var(--fg-dim)",
                border: "1px solid " + (title.trim() ? "var(--cyan)" : "var(--border)"),
                padding: "5px 14px",
                borderRadius: 6,
                cursor: saving || !title.trim() ? "default" : "pointer",
                fontSize: 11,
                fontWeight: 600,
                display: "flex",
                alignItems: "center",
                gap: 4,
              }}
            >
              <Save size={10} />
              {saving ? "saving…" : "save"}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

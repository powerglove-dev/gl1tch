/**
 * CollectorConfigModal — schema-driven structured editor for a
 * workspace's collectors.yaml. Renders the collector list down the
 * left rail with master enabled toggles, and the selected collector's
 * fields on the right via FieldInput.
 *
 * The modal speaks JSON to the backend (GetCollectorsConfigJSON /
 * WriteCollectorsConfigJSON), which round-trips through the typed
 * collector.Config struct. That gives us validated structured edits
 * at the cost of dropping YAML comments — power users who need
 * comments keep using EditorPopup's raw YAML route via the "Edit
 * raw YAML" link in the footer.
 *
 * Visual style mirrors EditorPopup so the two modals feel like
 * siblings (same backdrop blur, same Dracula border + bg-dark surface,
 * same footer button shapes).
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { X, Save, RotateCcw, FileCode, Settings } from "lucide-react";

import {
  COLLECTOR_SCHEMA,
  findCollectorById,
  getField,
  setField,
  type CollectorSpec,
  type FieldSpec,
} from "../../lib/collectorSchema";
import {
  getCollectorsConfigJSON,
  writeCollectorsConfigJSON,
} from "../../lib/collectorBridge";
import { FieldInput } from "./FieldInput";
import { useToast } from "../Toast";

interface Props {
  workspaceId: string;
  /** Optional collector id to pre-select on open. When the user
   *  clicks the per-row pencil in the brain popover, we route them
   *  straight to that collector's tab. */
  initialCollectorId?: string;
  onClose: () => void;
  /** Called after a successful save so the brain popover can
   *  refresh its collector list with new enabled/disabled state. */
  onSaved: () => void;
  /** Optional escape hatch — opens the existing raw-YAML EditorPopup
   *  on the same collectors.yaml file. The structured modal closes
   *  before opening it so the two never stack. */
  onEditRawYAML?: () => void;
}

type FormState = Record<string, unknown>;

export function CollectorConfigModal({
  workspaceId,
  initialCollectorId,
  onClose,
  onSaved,
  onEditRawYAML,
}: Props) {
  const toast = useToast();

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [original, setOriginal] = useState<FormState>({});
  const [values, setValues] = useState<FormState>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saveError, setSaveError] = useState<string>("");

  const [selectedId, setSelectedId] = useState<string>(
    initialCollectorId ?? COLLECTOR_SCHEMA[0].id,
  );

  // ── Load ────────────────────────────────────────────────────────────
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getCollectorsConfigJSON(workspaceId)
      .then((json) => {
        if (cancelled) return;
        try {
          const parsed = json ? JSON.parse(json) : {};
          setOriginal(parsed);
          setValues(parsed);
        } catch (e) {
          toast.error("Couldn't parse collectors config", { detail: String(e) });
          setOriginal({});
          setValues({});
        }
      })
      .catch((e) => {
        if (cancelled) return;
        toast.error("Couldn't load collectors config", { detail: String(e) });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // Intentionally only depends on workspaceId — toast is a stable
    // ref from the provider and re-running this effect on every parent
    // re-render would re-fetch the config.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId]);

  // ── Esc + Cmd+S keyboard shortcuts ──────────────────────────────────
  const dirty = useMemo(
    () => JSON.stringify(values) !== JSON.stringify(original),
    [values, original],
  );

  const handleSave = useCallback(async () => {
    if (saving || !dirty) return;
    setSaving(true);
    setSaveError("");
    try {
      const errMsg = await writeCollectorsConfigJSON(
        workspaceId,
        JSON.stringify(values),
      );
      if (errMsg) {
        setSaveError(errMsg);
        toast.error("Couldn't save collectors config", { detail: errMsg });
        return;
      }
      toast.success("Collectors config saved");
      setOriginal(values);
      onSaved();
    } catch (e) {
      const msg = String(e);
      setSaveError(msg);
      toast.error("Couldn't save collectors config", { detail: msg });
    } finally {
      setSaving(false);
    }
  }, [dirty, onSaved, saving, toast, values, workspaceId]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        void handleSave();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [handleSave, onClose]);

  // ── Field helpers ───────────────────────────────────────────────────
  const updateField = useCallback((key: string, value: unknown) => {
    setValues((prev) => setField(prev, key, value));
    // Clear the error for this field on edit; we re-validate on blur.
    setErrors((prev) => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const validateField = useCallback(
    (spec: FieldSpec, current: FormState): string => {
      const v = getField(current, spec.key);
      if (spec.required && (v == null || v === "")) {
        return `${spec.label} is required`;
      }
      if (spec.type === "number" && v != null && typeof v === "number") {
        if (spec.min != null && v < spec.min)
          return `${spec.label} must be ≥ ${spec.min}`;
        if (spec.max != null && v > spec.max)
          return `${spec.label} must be ≤ ${spec.max}`;
      }
      return "";
    },
    [],
  );

  const onFieldBlur = useCallback(
    (spec: FieldSpec) => {
      const err = validateField(spec, values);
      setErrors((prev) => {
        if (err === (prev[spec.key] ?? "")) return prev;
        const next = { ...prev };
        if (err) next[spec.key] = err;
        else delete next[spec.key];
        return next;
      });
    },
    [validateField, values],
  );

  const handleRevert = useCallback(() => {
    setValues(original);
    setErrors({});
    setSaveError("");
  }, [original]);

  const selected = findCollectorById(selectedId) ?? COLLECTOR_SCHEMA[0];

  // Toggle a collector's enabled flag from the sidebar without
  // navigating into it. For collectors with no enabledKey (git,
  // github, mattermost) the toggle is a no-op visually because
  // their "enabled" state is derived from list/credential presence.
  const toggleCollectorEnabled = useCallback(
    (spec: CollectorSpec) => {
      if (!spec.enabledKey) return;
      const current = !!getField(values, spec.enabledKey);
      updateField(spec.enabledKey, !current);
    },
    [updateField, values],
  );

  // ── Render ──────────────────────────────────────────────────────────
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
          width: "min(960px, calc(100% - 48px))",
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
          <Settings size={14} style={{ color: "var(--cyan)" }} />
          <span
            style={{
              flex: 1,
              fontSize: 13,
              fontWeight: 600,
              color: "var(--fg)",
            }}
          >
            Configure collectors
          </span>
          {dirty && (
            <span
              style={{
                fontSize: 9,
                textTransform: "uppercase",
                letterSpacing: "0.08em",
                color: "var(--yellow)",
                border: "1px solid var(--border)",
                padding: "2px 6px",
                borderRadius: 4,
              }}
            >
              unsaved
            </span>
          )}
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              color: "var(--fg-dim)",
              cursor: "pointer",
              padding: 4,
              display: "flex",
            }}
            title="Close (Esc)"
          >
            <X size={16} />
          </button>
        </div>

        {/* ── Body ─────────────────────────────────────────────────── */}
        <div style={{ flex: 1, display: "flex", overflow: "hidden" }}>
          {/* Sidebar */}
          <div
            style={{
              width: 220,
              borderRight: "1px solid var(--border)",
              overflowY: "auto",
              padding: "10px 0",
            }}
          >
            {COLLECTOR_SCHEMA.map((spec) => {
              const enabled = spec.isEnabled(values);
              const isSelected = spec.id === selectedId;
              return (
                <div
                  key={spec.id}
                  onClick={() => setSelectedId(spec.id)}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "8px 14px",
                    cursor: "pointer",
                    background: isSelected
                      ? "rgba(139, 233, 253, 0.08)"
                      : "transparent",
                    borderLeft: isSelected
                      ? "2px solid var(--cyan)"
                      : "2px solid transparent",
                  }}
                >
                  <span
                    style={{
                      width: 6,
                      height: 6,
                      borderRadius: 999,
                      background: enabled ? "var(--green)" : "var(--fg-dim)",
                      boxShadow: enabled ? "0 0 6px var(--green)" : "none",
                      flexShrink: 0,
                    }}
                  />
                  <span
                    style={{
                      flex: 1,
                      fontSize: 12,
                      fontWeight: isSelected ? 600 : 500,
                      color: enabled ? "var(--fg)" : "var(--fg-dim)",
                    }}
                  >
                    {spec.name}
                  </span>
                  {spec.enabledKey && (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        toggleCollectorEnabled(spec);
                      }}
                      title={enabled ? "Disable" : "Enable"}
                      style={{
                        background: "none",
                        border: "none",
                        cursor: "pointer",
                        padding: 0,
                        display: "flex",
                      }}
                    >
                      <span
                        style={{
                          width: 22,
                          height: 12,
                          borderRadius: 999,
                          background: enabled ? "var(--cyan)" : "var(--bg)",
                          border: "1px solid var(--border)",
                          position: "relative",
                          display: "block",
                        }}
                      >
                        <span
                          style={{
                            position: "absolute",
                            top: 0,
                            left: enabled ? 11 : 1,
                            width: 9,
                            height: 9,
                            borderRadius: 999,
                            background: enabled
                              ? "var(--bg-dark)"
                              : "var(--fg-dim)",
                            transition: "left 0.15s",
                          }}
                        />
                      </span>
                    </button>
                  )}
                </div>
              );
            })}
          </div>

          {/* Main pane */}
          <div
            style={{
              flex: 1,
              overflowY: "auto",
              padding: "18px 24px 24px",
            }}
          >
            {loading ? (
              <div
                style={{
                  color: "var(--fg-dim)",
                  fontSize: 12,
                  fontStyle: "italic",
                }}
              >
                Loading…
              </div>
            ) : (
              <>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    marginBottom: 6,
                  }}
                >
                  <h2
                    style={{
                      margin: 0,
                      fontSize: 16,
                      fontWeight: 600,
                      color: "var(--fg)",
                    }}
                  >
                    {selected.name}
                  </h2>
                </div>
                <p
                  style={{
                    margin: "0 0 18px",
                    fontSize: 12,
                    lineHeight: 1.6,
                    color: "var(--fg-dim)",
                  }}
                >
                  {selected.description}
                </p>

                <div
                  style={{
                    display: "flex",
                    flexDirection: "column",
                    gap: 18,
                  }}
                >
                  {selected.fields
                    .filter((f) => !f.visibleWhen || f.visibleWhen(values))
                    .map((spec) => (
                      <FieldInput
                        key={spec.key}
                        spec={spec}
                        value={getField(values, spec.key)}
                        error={errors[spec.key] ?? ""}
                        onChange={(v) => updateField(spec.key, v)}
                        onBlur={() => onFieldBlur(spec)}
                      />
                    ))}
                </div>
              </>
            )}
          </div>
        </div>

        {/* ── Footer ─────────────────────────────────────────────── */}
        <div
          style={{
            padding: "10px 18px",
            borderTop: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            gap: 10,
            background: "var(--bg-dark)",
          }}
        >
          {saveError ? (
            <span
              style={{
                flex: 1,
                fontSize: 11,
                color: "var(--orange, #ffb86c)",
                fontFamily: "monospace",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
              title={saveError}
            >
              {saveError}
            </span>
          ) : (
            <span style={{ flex: 1 }} />
          )}

          {onEditRawYAML && (
            <button
              type="button"
              onClick={() => {
                onEditRawYAML();
                onClose();
              }}
              title="Open the raw YAML editor"
              style={footerSecondaryButtonStyle}
            >
              <FileCode size={11} />
              Edit raw YAML
            </button>
          )}

          <button
            type="button"
            onClick={handleRevert}
            disabled={!dirty}
            style={{
              ...footerSecondaryButtonStyle,
              opacity: dirty ? 1 : 0.4,
              cursor: dirty ? "pointer" : "not-allowed",
            }}
          >
            <RotateCcw size={11} />
            Revert
          </button>

          <button
            type="button"
            onClick={handleSave}
            disabled={!dirty || saving}
            style={{
              ...footerPrimaryButtonStyle,
              opacity: dirty && !saving ? 1 : 0.4,
              cursor: dirty && !saving ? "pointer" : "not-allowed",
            }}
          >
            <Save size={11} />
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

const footerSecondaryButtonStyle: React.CSSProperties = {
  background: "none",
  border: "1px solid var(--border)",
  borderRadius: 6,
  padding: "5px 12px",
  fontSize: 11,
  fontWeight: 600,
  color: "var(--fg)",
  cursor: "pointer",
  display: "flex",
  alignItems: "center",
  gap: 5,
};

const footerPrimaryButtonStyle: React.CSSProperties = {
  ...footerSecondaryButtonStyle,
  borderColor: "var(--cyan)",
  color: "var(--cyan)",
};

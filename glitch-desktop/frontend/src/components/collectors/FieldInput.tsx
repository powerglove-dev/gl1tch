/**
 * FieldInput — generic, schema-driven field renderer for the collector
 * config modal. Dispatches on FieldSpec.type and renders the matching
 * control. New field types are added by extending the switch below
 * and updating the FieldType union in lib/collectorSchema.ts.
 *
 * Validation is per-field and runs on blur (matches ensemble's
 * ParamInput pattern — keeps regex and duration parsing off the
 * keystroke path). The parent modal owns the form state; this
 * component is presentational and only emits onChange.
 */
import { useMemo, useState } from "react";
import { Plus, X, KeyRound } from "lucide-react";
import type { FieldSpec } from "../../lib/collectorSchema";
import { parseDuration, formatDuration } from "../../lib/collectorSchema";

interface Props {
  spec: FieldSpec;
  /** Current value pulled out of the form state via getField(). */
  value: unknown;
  /** Inline validation error string, or "". */
  error: string;
  onChange: (value: unknown) => void;
  /** Called when focus leaves the control. The parent uses this to
   *  re-run validation for the field, mirroring ensemble's blur-only
   *  validation strategy. */
  onBlur: () => void;
}

export function FieldInput({ spec, value, error, onChange, onBlur }: Props) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <label
        style={{
          fontSize: 11,
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.06em",
          color: "var(--fg)",
        }}
      >
        {spec.label}
      </label>
      {spec.description && (
        <p
          style={{
            margin: 0,
            fontSize: 11,
            lineHeight: 1.5,
            color: "var(--fg-dim)",
          }}
        >
          {spec.description}
        </p>
      )}

      <FieldControl
        spec={spec}
        value={value}
        // Read-only fields no-op their onChange so accidental edits
        // (or stale form state) can't dirty the form for fields the
        // user can't actually persist.
        onChange={spec.readOnly ? () => {} : onChange}
        onBlur={onBlur}
        readOnly={!!spec.readOnly}
      />

      {error && (
        <p
          style={{
            margin: 0,
            fontSize: 11,
            color: "var(--orange, #ffb86c)",
          }}
        >
          {error}
        </p>
      )}
    </div>
  );
}

function FieldControl({
  spec,
  value,
  onChange,
  onBlur,
  readOnly,
}: {
  spec: FieldSpec;
  value: unknown;
  onChange: (v: unknown) => void;
  onBlur: () => void;
  readOnly: boolean;
}) {
  if (readOnly) {
    return <ReadOnlyDisplay spec={spec} value={value} />;
  }
  switch (spec.type) {
    case "boolean":
      return (
        <BooleanField value={!!value} onChange={onChange} onBlur={onBlur} />
      );
    case "string":
      return (
        <TextField
          value={(value as string) ?? ""}
          onChange={onChange}
          onBlur={onBlur}
          placeholder={spec.placeholder}
          type="text"
        />
      );
    case "secret":
      return (
        <TextField
          value={(value as string) ?? ""}
          onChange={onChange}
          onBlur={onBlur}
          placeholder={spec.placeholder}
          type="password"
          icon={<KeyRound size={12} />}
        />
      );
    case "number":
      return (
        <NumberField
          value={value as number | undefined}
          onChange={onChange}
          onBlur={onBlur}
          min={spec.min}
          max={spec.max}
          placeholder={spec.placeholder}
        />
      );
    case "duration":
      return (
        <DurationField
          value={value as number | undefined}
          onChange={onChange}
          onBlur={onBlur}
          placeholder={spec.placeholder}
        />
      );
    case "string-list":
      return (
        <StringListField
          value={(value as string[]) ?? []}
          onChange={onChange}
          onBlur={onBlur}
          placeholder={spec.placeholder}
        />
      );
    case "path-list":
      return (
        <StringListField
          value={(value as string[]) ?? []}
          onChange={onChange}
          onBlur={onBlur}
          placeholder={spec.placeholder ?? "/absolute/path"}
        />
      );
    case "enum":
      return (
        <EnumField
          value={(value as string) ?? ""}
          onChange={onChange}
          onBlur={onBlur}
          options={spec.enumValues ?? []}
        />
      );
  }
}

// ── Read-only display ──────────────────────────────────────────────────
//
// Used for fields whose value is auto-derived elsewhere (e.g.
// git.repos comes from workspace directories via AutoDetectFromWorkspace).
// We render a chip list / value strip rather than disabled inputs so
// it's visually obvious nothing here is editable, while still showing
// the current values.

function ReadOnlyDisplay({
  spec,
  value,
}: {
  spec: FieldSpec;
  value: unknown;
}) {
  if (
    spec.type === "string-list" ||
    spec.type === "path-list"
  ) {
    const items = (value as string[] | undefined) ?? [];
    if (items.length === 0) {
      return (
        <div
          style={{
            fontSize: 11,
            fontStyle: "italic",
            color: "var(--fg-dim)",
            padding: "5px 10px",
            border: "1px dashed var(--border)",
            borderRadius: 6,
          }}
        >
          (none yet)
        </div>
      );
    }
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {items.map((item, i) => (
          <div
            key={i}
            style={{
              padding: "5px 10px",
              background: "var(--bg)",
              border: "1px solid var(--border)",
              borderRadius: 6,
              fontSize: 12,
              color: "var(--fg-dim)",
              fontFamily: "Berkeley Mono, JetBrains Mono, SF Mono, monospace",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              opacity: 0.8,
            }}
          >
            {item}
          </div>
        ))}
      </div>
    );
  }
  // Fallback for scalar read-only fields.
  const display =
    value == null || value === ""
      ? "(unset)"
      : typeof value === "boolean"
        ? value
          ? "Enabled"
          : "Disabled"
        : String(value);
  return (
    <div
      style={{
        padding: "5px 10px",
        background: "var(--bg)",
        border: "1px solid var(--border)",
        borderRadius: 6,
        fontSize: 12,
        color: "var(--fg-dim)",
        opacity: 0.8,
      }}
    >
      {display}
    </div>
  );
}

// ── Sub-controls ───────────────────────────────────────────────────────

function BooleanField({
  value,
  onChange,
  onBlur,
}: {
  value: boolean;
  onChange: (v: boolean) => void;
  onBlur: () => void;
}) {
  return (
    <label
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 8,
        cursor: "pointer",
        userSelect: "none",
      }}
      onBlur={onBlur}
    >
      <span
        onClick={() => onChange(!value)}
        style={{
          width: 30,
          height: 16,
          borderRadius: 999,
          background: value ? "var(--cyan)" : "var(--bg)",
          border: "1px solid var(--border)",
          position: "relative",
          transition: "background 0.15s",
        }}
      >
        <span
          style={{
            position: "absolute",
            top: 1,
            left: value ? 15 : 1,
            width: 12,
            height: 12,
            borderRadius: 999,
            background: value ? "var(--bg-dark)" : "var(--fg-dim)",
            transition: "left 0.15s",
          }}
        />
      </span>
      <span style={{ fontSize: 11, color: "var(--fg-dim)" }}>
        {value ? "Enabled" : "Disabled"}
      </span>
    </label>
  );
}

const inputBaseStyle: React.CSSProperties = {
  background: "var(--bg)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  padding: "6px 10px",
  fontSize: 12,
  color: "var(--fg)",
  fontFamily: "inherit",
  outline: "none",
  width: "100%",
  boxSizing: "border-box",
};

function TextField({
  value,
  onChange,
  onBlur,
  placeholder,
  type = "text",
  icon,
}: {
  value: string;
  onChange: (v: string) => void;
  onBlur: () => void;
  placeholder?: string;
  type?: "text" | "password";
  icon?: React.ReactNode;
}) {
  return (
    <div style={{ position: "relative" }}>
      {icon && (
        <span
          style={{
            position: "absolute",
            left: 8,
            top: "50%",
            transform: "translateY(-50%)",
            color: "var(--fg-dim)",
            display: "flex",
          }}
        >
          {icon}
        </span>
      )}
      <input
        type={type}
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
        style={{ ...inputBaseStyle, paddingLeft: icon ? 26 : 10 }}
      />
    </div>
  );
}

function NumberField({
  value,
  onChange,
  onBlur,
  min,
  max,
  placeholder,
}: {
  value: number | undefined;
  onChange: (v: number | undefined) => void;
  onBlur: () => void;
  min?: number;
  max?: number;
  placeholder?: string;
}) {
  return (
    <input
      type="number"
      value={value ?? ""}
      min={min}
      max={max}
      placeholder={placeholder}
      onChange={(e) => {
        const raw = e.target.value;
        if (raw === "") {
          onChange(undefined);
          return;
        }
        const n = parseInt(raw, 10);
        onChange(Number.isFinite(n) ? n : undefined);
      }}
      onBlur={onBlur}
      style={inputBaseStyle}
    />
  );
}

function DurationField({
  value,
  onChange,
  onBlur,
  placeholder,
}: {
  // Wire format is nanoseconds (Go time.Duration → JSON int64).
  value: number | undefined;
  onChange: (v: number | undefined) => void;
  onBlur: () => void;
  placeholder?: string;
}) {
  // Local string state so the user can type "30m" without us
  // round-tripping every keystroke through the duration parser.
  const [text, setText] = useState(() => formatDuration(value));

  // Re-sync from props when the parent revert wipes the form.
  useMemo(() => {
    setText(formatDuration(value));
    // Re-run when value changes from outside the component.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);

  return (
    <input
      type="text"
      value={text}
      placeholder={placeholder}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => {
        const ns = parseDuration(text);
        if (ns != null) {
          onChange(ns);
          setText(formatDuration(ns));
        }
        onBlur();
      }}
      style={inputBaseStyle}
    />
  );
}

function EnumField({
  value,
  onChange,
  onBlur,
  options,
}: {
  value: string;
  onChange: (v: string) => void;
  onBlur: () => void;
  options: string[];
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      onBlur={onBlur}
      style={{ ...inputBaseStyle, cursor: "pointer" }}
    >
      {!value && <option value="">— select —</option>}
      {options.map((opt) => (
        <option key={opt} value={opt}>
          {opt}
        </option>
      ))}
    </select>
  );
}

function StringListField({
  value,
  onChange,
  onBlur,
  placeholder,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  onBlur: () => void;
  placeholder?: string;
}) {
  const [draft, setDraft] = useState("");

  function add() {
    const v = draft.trim();
    if (!v) return;
    onChange([...(value ?? []), v]);
    setDraft("");
  }

  function remove(i: number) {
    const next = [...(value ?? [])];
    next.splice(i, 1);
    onChange(next);
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      {(value ?? []).map((item, i) => (
        <div
          key={i}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 6,
            padding: "5px 10px",
            background: "var(--bg)",
            border: "1px solid var(--border)",
            borderRadius: 6,
            fontSize: 12,
            color: "var(--fg)",
          }}
        >
          <span
            style={{
              flex: 1,
              fontFamily: "Berkeley Mono, JetBrains Mono, SF Mono, monospace",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {item}
          </span>
          <button
            type="button"
            onClick={() => remove(i)}
            title="Remove"
            style={{
              background: "none",
              border: "none",
              color: "var(--fg-dim)",
              cursor: "pointer",
              padding: 2,
              display: "flex",
            }}
          >
            <X size={12} />
          </button>
        </div>
      ))}
      <div style={{ display: "flex", gap: 6 }}>
        <input
          type="text"
          value={draft}
          placeholder={placeholder}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          onBlur={onBlur}
          style={{ ...inputBaseStyle, flex: 1 }}
        />
        <button
          type="button"
          onClick={add}
          disabled={!draft.trim()}
          style={{
            background: "none",
            border: "1px solid var(--border)",
            borderRadius: 6,
            padding: "5px 10px",
            color: "var(--cyan)",
            cursor: draft.trim() ? "pointer" : "not-allowed",
            opacity: draft.trim() ? 1 : 0.4,
            display: "flex",
            alignItems: "center",
            gap: 4,
            fontSize: 11,
            fontWeight: 600,
          }}
        >
          <Plus size={12} />
          Add
        </button>
      </div>
    </div>
  );
}

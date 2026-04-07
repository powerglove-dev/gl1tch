/**
 * Compact provider/model picker dropdown.
 *
 * Extracted from ChatInput so the editor popup's chat strip can reuse
 * the exact same picker UI. The picker is a controlled component —
 * the parent owns selectedProvider/selectedModel state and decides
 * whether to "stick" the selection or snap back after each turn.
 *
 * Selecting "observer" means "no explicit override; route through the
 * observer's pinned default model". The star button on each model row
 * pins that model as the observer default (persisted via the parent's
 * onSetObserverDefault callback).
 */
import { useEffect, useRef, useState } from "react";
import { ChevronDown, Cpu } from "lucide-react";

export interface PickerProviderOption {
  id: string;
  label: string;
  models: { id: string; label: string; default: boolean }[];
}

interface Props {
  providers: PickerProviderOption[];
  /** Empty string = observer mode (no explicit override). */
  selectedProvider: string;
  selectedModel: string;
  /** Provider id used as the executor when the picker is on "observer". */
  observerDefaultProvider: string;
  /** Model id used as the executor when the picker is on "observer". */
  observerDefaultModel: string;
  onSelectProvider: (providerId: string, modelId: string) => void;
  /** Persist the (providerId, modelId) pair as the observer default. */
  onSetObserverDefault: (providerId: string, modelId: string) => void;
  /** Optional alignment hint — defaults to "right" for chat input contexts. */
  align?: "left" | "right";
}

export function ProviderPicker({
  providers,
  selectedProvider,
  selectedModel,
  observerDefaultProvider,
  observerDefaultModel,
  onSelectProvider,
  onSetObserverDefault,
  align = "right",
}: Props) {
  const buttonRef = useRef<HTMLButtonElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);

  // Close on outside click / Escape — same behavior as ChatInput's
  // original picker. Without this the dropdown sticks open until you
  // re-click the toggle.
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      const t = e.target as Node | null;
      if (!t) return;
      if (dropdownRef.current?.contains(t)) return;
      if (buttonRef.current?.contains(t)) return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const currentProvider = providers.find((p) => p.id === selectedProvider);
  const providerLabel = currentProvider?.label ?? "observer";
  const observerDefaultProviderObj = providers.find((p) => p.id === observerDefaultProvider);
  const observerDefaultModelLabel =
    observerDefaultProviderObj?.models.find((m) => m.id === observerDefaultModel)?.label ||
    observerDefaultModel ||
    "auto";

  const dropdownStyle: React.CSSProperties = {
    position: "absolute",
    bottom: "100%",
    marginBottom: 4,
    background: "var(--bg-dark)",
    border: "1px solid var(--border-bright)",
    borderRadius: 8,
    padding: 4,
    minWidth: 240,
    maxHeight: 320,
    overflowY: "auto",
    zIndex: 100,
    boxShadow: "0 4px 16px rgba(0,0,0,0.4)",
  };
  if (align === "right") dropdownStyle.right = 0;
  else dropdownStyle.left = 0;

  return (
    <div style={{ position: "relative", display: "inline-block", flexShrink: 0 }}>
      <button
        ref={buttonRef}
        onClick={() => setOpen(!open)}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 3,
          padding: "5px 7px",
          borderRadius: 8,
          fontSize: 10,
          background: "transparent",
          border: "1px solid var(--border)",
          color: "var(--fg-dim)",
          cursor: "pointer",
          whiteSpace: "nowrap",
        }}
        title={`${providerLabel} / ${selectedModel || "default"}`}
      >
        <Cpu size={10} style={{ color: "var(--cyan)" }} />
        <span style={{ color: "var(--cyan)" }}>{providerLabel}</span>
        <ChevronDown size={9} />
      </button>

      {open && (
        <div ref={dropdownRef} style={dropdownStyle}>
          <button
            onClick={() => {
              onSelectProvider("", "");
              setOpen(false);
            }}
            style={{
              width: "100%",
              display: "flex",
              alignItems: "center",
              gap: 6,
              padding: "6px 8px",
              borderRadius: 4,
              fontSize: 12,
              background: !selectedProvider ? "var(--bg-surface)" : "transparent",
              border: "none",
              color: "var(--fg)",
              cursor: "pointer",
              textAlign: "left",
            }}
            title={`observer routes prompts to ${observerDefaultProvider || "the first installed model"}`}
          >
            <span style={{ color: "var(--cyan)", fontWeight: 600 }}>observer</span>
            <span style={{ color: "var(--fg-dim)", fontSize: 10, marginLeft: "auto" }}>
              → {observerDefaultModelLabel}
            </span>
          </button>

          {providers.map((p) => (
            <div key={p.id}>
              <div
                style={{
                  padding: "6px 8px 2px",
                  fontSize: 10,
                  color: "var(--fg-dim)",
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                }}
              >
                {p.label}
              </div>
              {p.models.map((m) => {
                const isObserverDefault =
                  observerDefaultProvider === p.id && observerDefaultModel === m.id;
                const isSelected = selectedProvider === p.id && selectedModel === m.id;
                return (
                  <div
                    key={m.id}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      borderRadius: 4,
                      background: isSelected ? "var(--bg-surface)" : "transparent",
                    }}
                  >
                    <button
                      onClick={() => {
                        onSelectProvider(p.id, m.id);
                        setOpen(false);
                      }}
                      style={{
                        flex: 1,
                        display: "flex",
                        alignItems: "center",
                        gap: 6,
                        padding: "4px 4px 4px 16px",
                        fontSize: 12,
                        background: "transparent",
                        border: "none",
                        color: "var(--fg)",
                        cursor: "pointer",
                        textAlign: "left",
                      }}
                    >
                      {m.label || m.id}
                      {m.default && (
                        <span style={{ fontSize: 9, color: "var(--green)", marginLeft: 4 }}>
                          default
                        </span>
                      )}
                    </button>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onSetObserverDefault(p.id, m.id);
                      }}
                      title={
                        isObserverDefault
                          ? "current observer default"
                          : "pin as observer default"
                      }
                      style={{
                        padding: "4px 8px",
                        background: "transparent",
                        border: "none",
                        cursor: "pointer",
                        color: isObserverDefault ? "var(--yellow)" : "var(--fg-dim)",
                        fontSize: 12,
                        lineHeight: 1,
                      }}
                      onMouseEnter={(e) => (e.currentTarget.style.color = "var(--yellow)")}
                      onMouseLeave={(e) =>
                        (e.currentTarget.style.color = isObserverDefault
                          ? "var(--yellow)"
                          : "var(--fg-dim)")
                      }
                    >
                      {isObserverDefault ? "★" : "☆"}
                    </button>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

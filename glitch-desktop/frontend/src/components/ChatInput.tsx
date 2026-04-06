import { useRef, useCallback, useState } from "react";
import { ArrowUp, ChevronDown, Cpu, X } from "lucide-react";

export interface ProviderOption {
  id: string;
  label: string;
  models: { id: string; label: string; default: boolean }[];
}

interface Props {
  disabled: boolean;
  providers: ProviderOption[];
  selectedProvider: string;
  selectedModel: string;
  selectedAgent: string | null;
  onSelectProvider: (providerId: string, modelId: string) => void;
  onClearAgent: () => void;
  onSend: (text: string) => void;
}

export function ChatInput({
  disabled, providers, selectedProvider, selectedModel, selectedAgent,
  onSelectProvider, onClearAgent, onSend,
}: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const [pickerOpen, setPickerOpen] = useState(false);

  const handleSend = useCallback(() => {
    if (!ref.current) return;
    const val = ref.current.value.trim();
    if (!val || disabled) return;
    onSend(val);
    ref.current.value = "";
    ref.current.style.height = "";
  }, [disabled, onSend]);

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  function handleInput() {
    if (!ref.current) return;
    ref.current.style.height = "";
    ref.current.style.height = Math.min(ref.current.scrollHeight, 140) + "px";
  }

  const currentProvider = providers.find((p) => p.id === selectedProvider);
  const providerLabel = currentProvider?.label ?? "observer";
  const modelLabel = selectedModel || "default";

  return (
    <div style={{ borderTop: "1px solid var(--border)", background: "var(--bg-dark)", padding: "8px 16px" }}>
      <div style={{ maxWidth: 760, margin: "0 auto", marginBottom: 6, display: "flex", alignItems: "center", gap: 6 }}>
        {/* Active agent pill */}
        {selectedAgent && (
          <div
            style={{
              display: "flex", alignItems: "center", gap: 4,
              padding: "3px 8px", borderRadius: 6, fontSize: 11,
              background: "var(--purple)", color: "var(--bg-dark)",
              fontWeight: 600,
            }}
          >
            <span>@{selectedAgent}</span>
            <button
              onClick={onClearAgent}
              style={{ background: "none", border: "none", color: "var(--bg-dark)", cursor: "pointer", display: "flex", padding: 0 }}
            >
              <X size={10} />
            </button>
          </div>
        )}

        {/* Provider/model picker */}
        <div style={{ position: "relative", display: "inline-block" }}>
          <button
            onClick={() => setPickerOpen(!pickerOpen)}
            style={{
              display: "flex", alignItems: "center", gap: 4,
              padding: "3px 8px", borderRadius: 6, fontSize: 11,
              background: selectedProvider ? "var(--bg-surface)" : "transparent",
              border: "1px solid var(--border)",
              color: "var(--fg-dim)", cursor: "pointer",
            }}
          >
            <Cpu size={10} />
            <span style={{ color: "var(--cyan)" }}>{providerLabel}</span>
            <span style={{ opacity: 0.5 }}>/</span>
            <span>{modelLabel}</span>
            <ChevronDown size={10} />
          </button>

          {pickerOpen && (
            <div style={{
              position: "absolute", bottom: "100%", left: 0, marginBottom: 4,
              background: "var(--bg-dark)", border: "1px solid var(--border-bright)",
              borderRadius: 8, padding: 4, minWidth: 240, maxHeight: 300, overflowY: "auto",
              zIndex: 100, boxShadow: "0 4px 16px rgba(0,0,0,0.4)",
            }}>
              {/* Observer (default) */}
              <button
                onClick={() => { onSelectProvider("", ""); setPickerOpen(false); }}
                style={{
                  width: "100%", display: "flex", alignItems: "center", gap: 6,
                  padding: "6px 8px", borderRadius: 4, fontSize: 12,
                  background: !selectedProvider ? "var(--bg-surface)" : "transparent",
                  border: "none", color: "var(--fg)", cursor: "pointer", textAlign: "left",
                }}
              >
                <span style={{ color: "var(--cyan)", fontWeight: 600 }}>observer</span>
                <span style={{ color: "var(--fg-dim)", fontSize: 10 }}>ES + Ollama</span>
              </button>

              {providers.map((p) => (
                <div key={p.id}>
                  <div style={{ padding: "6px 8px 2px", fontSize: 10, color: "var(--fg-dim)", textTransform: "uppercase", letterSpacing: "0.05em" }}>
                    {p.label}
                  </div>
                  {p.models.map((m) => (
                    <button
                      key={m.id}
                      onClick={() => { onSelectProvider(p.id, m.id); setPickerOpen(false); }}
                      style={{
                        width: "100%", display: "flex", alignItems: "center", gap: 6,
                        padding: "4px 8px 4px 16px", borderRadius: 4, fontSize: 12,
                        background: selectedProvider === p.id && selectedModel === m.id ? "var(--bg-surface)" : "transparent",
                        border: "none", color: "var(--fg)", cursor: "pointer", textAlign: "left",
                      }}
                    >
                      {m.label || m.id}
                      {m.default && <span style={{ fontSize: 9, color: "var(--green)", marginLeft: 4 }}>default</span>}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Input area */}
      <div style={{
        display: "flex", alignItems: "flex-end", gap: 8,
        maxWidth: 760, margin: "0 auto",
        background: "var(--bg)", border: "1px solid var(--border)",
        borderRadius: 12, padding: "4px 4px 4px 12px",
      }}>
        <textarea
          ref={ref} rows={1}
          onKeyDown={handleKeyDown} onInput={handleInput}
          disabled={disabled} autoFocus
          placeholder="Ask about your repos, agents, and activity..."
          style={{
            flex: 1, background: "none", border: "none", color: "var(--fg)",
            fontSize: 13, fontFamily: "-apple-system, system-ui, sans-serif",
            resize: "none", maxHeight: 140, overflowY: "auto", outline: "none",
            lineHeight: 1.5, padding: "6px 0",
          }}
        />
        <button
          onClick={handleSend} disabled={disabled}
          style={{
            width: 30, height: 30, borderRadius: 8,
            background: disabled ? "var(--bg-surface)" : "var(--cyan)",
            border: "none", color: disabled ? "var(--fg-dim)" : "var(--bg-dark)",
            cursor: disabled ? "default" : "pointer",
            display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0,
          }}
        >
          <ArrowUp size={15} strokeWidth={2.5} />
        </button>
      </div>
    </div>
  );
}

import { useRef, useCallback, useState, useEffect } from "react";
import { ArrowUp, ChevronDown, Cpu, X, ChevronRight, FileText, Play, Bot, Save } from "lucide-react";
import { GetWorkflowFileDetails } from "../../wailsjs/go/main/App";

interface WorkflowStepInfo {
  id: string;
  executor: string;
  model: string;
  prompt_preview: string;
  needs?: string[];
}

interface WorkflowFileDetails {
  name: string;
  description: string;
  path: string;
  steps: WorkflowStepInfo[];
}

export interface ProviderOption {
  id: string;
  label: string;
  models: { id: string; label: string; default: boolean }[];
}

/** A single step in the execution chain.
 *  executorOverride/modelOverride let a step run with a different provider
 *  than the one selected globally in the picker. */
export type ChainStep =
  | { type: "prompt"; id: number; label: string; body: string; executorOverride?: string; modelOverride?: string }
  | { type: "agent"; name: string; label: string; kind: string; invoke: string }
  | { type: "pipeline"; path: string; label: string; description?: string };

interface Props {
  disabled: boolean;
  providers: ProviderOption[];
  selectedProvider: string;
  selectedModel: string;
  chain: ChainStep[];
  onSelectProvider: (providerId: string, modelId: string) => void;
  onUpdateChainStep: (index: number, step: ChainStep) => void;
  onRemoveChainStep: (index: number) => void;
  onClearChain: () => void;
  onSaveWorkflow: (name: string) => void;
  onSend: (text: string) => void;
}

function StepEditor({
  step, providers, onSave, onClose,
}: {
  step: ChainStep;
  providers: ProviderOption[];
  onSave: (s: ChainStep) => void;
  onClose: () => void;
}) {
  const [label, setLabel] = useState(step.label);
  const [execOverride, setExecOverride] = useState(
    step.type === "prompt" ? step.executorOverride ?? "" : "",
  );
  const [modelOverride, setModelOverride] = useState(
    step.type === "prompt" ? step.modelOverride ?? "" : "",
  );
  const [details, setDetails] = useState<WorkflowFileDetails | null>(null);
  const [loadingDetails, setLoadingDetails] = useState(false);

  // For pipeline (workflow file) steps, fetch the YAML metadata so we can
  // show what the workflow does and what steps it will run.
  useEffect(() => {
    if (step.type === "pipeline" && step.path) {
      setLoadingDetails(true);
      GetWorkflowFileDetails(step.path)
        .then((json) => {
          try {
            const parsed = JSON.parse(json) as WorkflowFileDetails;
            setDetails(parsed);
          } catch {}
        })
        .finally(() => setLoadingDetails(false));
    }
  }, [step]);

  function commit() {
    if (step.type === "prompt") {
      onSave({ ...step, label, executorOverride: execOverride || undefined, modelOverride: modelOverride || undefined });
    } else {
      onSave({ ...step, label });
    }
    onClose();
  }

  const currentProvider = providers.find((p) => p.id === execOverride);

  // Color-code executor names so users can scan a workflow at a glance.
  function executorColor(executor: string): string {
    if (executor === "shell") return "var(--yellow)";
    if (executor === "ollama") return "var(--cyan)";
    if (executor.startsWith("builtin")) return "var(--fg-dim)";
    if (executor === "claude" || executor === "github-copilot" || executor === "opencode") return "var(--purple)";
    return "var(--green)";
  }

  return (
    <div
      onClick={(e) => e.stopPropagation()}
      style={{
        position: "absolute", bottom: "100%", left: 0, marginBottom: 6,
        background: "var(--bg-dark)", border: "1px solid var(--border-bright)",
        borderRadius: 10, padding: 12, minWidth: 320, maxWidth: 420,
        zIndex: 200, boxShadow: "0 6px 24px rgba(0,0,0,0.5)",
      }}
    >
      <div style={{ fontSize: 10, fontWeight: 600, color: "var(--fg-dim)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 6 }}>
        {step.type === "pipeline" ? "Workflow details" : "Edit step"}
      </div>
      <input
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        autoFocus
        onKeyDown={(e) => { if (e.key === "Enter") commit(); if (e.key === "Escape") onClose(); }}
        placeholder="Label"
        style={{
          width: "100%", padding: "6px 10px", marginBottom: 8,
          background: "var(--bg)", border: "1px solid var(--border)",
          borderRadius: 6, color: "var(--fg)", fontSize: 12,
          outline: "none", fontFamily: "inherit",
        }}
      />

      {step.type === "prompt" && (
        <>
          <div style={{ fontSize: 10, color: "var(--fg-dim)", marginBottom: 4 }}>
            Executor override (optional)
          </div>
          <select
            value={execOverride}
            onChange={(e) => { setExecOverride(e.target.value); setModelOverride(""); }}
            style={{
              width: "100%", padding: "6px 8px", marginBottom: 6,
              background: "var(--bg)", border: "1px solid var(--border)",
              borderRadius: 6, color: "var(--fg)", fontSize: 12,
              outline: "none", fontFamily: "inherit",
            }}
          >
            <option value="">Use default (from chat picker)</option>
            {providers.map((p) => (
              <option key={p.id} value={p.id}>{p.label}</option>
            ))}
          </select>
          {currentProvider && (
            <select
              value={modelOverride}
              onChange={(e) => setModelOverride(e.target.value)}
              style={{
                width: "100%", padding: "6px 8px", marginBottom: 6,
                background: "var(--bg)", border: "1px solid var(--border)",
                borderRadius: 6, color: "var(--fg)", fontSize: 12,
                outline: "none", fontFamily: "inherit",
              }}
            >
              <option value="">Default model</option>
              {currentProvider.models.map((m) => (
                <option key={m.id} value={m.id}>{m.label || m.id}</option>
              ))}
            </select>
          )}
        </>
      )}

      {step.type === "pipeline" && (
        <>
          {/* Description (immediate from chain step) */}
          {(step.description || details?.description) && (
            <div style={{
              fontSize: 11, color: "var(--fg)",
              padding: "8px 10px", marginBottom: 8,
              background: "var(--bg)", borderRadius: 6,
              border: "1px solid var(--border)",
              lineHeight: 1.4,
            }}>
              {details?.description || step.description}
            </div>
          )}

          {/* Inner steps */}
          <div style={{ fontSize: 10, color: "var(--fg-dim)", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
            Steps {details && `· ${details.steps.length}`}
          </div>
          <div style={{
            maxHeight: 220, overflowY: "auto",
            background: "var(--bg)", border: "1px solid var(--border)",
            borderRadius: 6, padding: 6, marginBottom: 8,
          }}>
            {loadingDetails && (
              <div style={{ fontSize: 11, color: "var(--fg-dim)", padding: "6px 4px" }}>Loading...</div>
            )}
            {!loadingDetails && details && details.steps.length === 0 && (
              <div style={{ fontSize: 11, color: "var(--fg-dim)", padding: "6px 4px" }}>No steps</div>
            )}
            {!loadingDetails && details && details.steps.map((s, i) => (
              <div key={s.id} style={{
                display: "flex", flexDirection: "column", gap: 2,
                padding: "5px 6px",
                borderTop: i > 0 ? "1px solid var(--border)" : "none",
              }}>
                <div style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11 }}>
                  <span style={{ color: "var(--fg-dim)", fontVariantNumeric: "tabular-nums", minWidth: 14 }}>
                    {i + 1}.
                  </span>
                  <span style={{ color: "var(--fg-bright)", fontWeight: 500 }}>{s.id}</span>
                  <span style={{
                    fontSize: 9, padding: "1px 5px", borderRadius: 3,
                    background: executorColor(s.executor) + "22",
                    color: executorColor(s.executor),
                    fontWeight: 600,
                  }}>
                    {s.executor}
                  </span>
                  {s.model && (
                    <span style={{ fontSize: 9, color: "var(--fg-dim)" }}>{s.model}</span>
                  )}
                </div>
                {s.prompt_preview && (
                  <div style={{
                    fontSize: 10, color: "var(--fg-dim)",
                    paddingLeft: 20, lineHeight: 1.4,
                    overflow: "hidden", textOverflow: "ellipsis",
                  }}>
                    {s.prompt_preview}
                  </div>
                )}
              </div>
            ))}
          </div>

          {/* Path (small/dim) */}
          {details && (
            <div style={{
              fontSize: 9, color: "var(--fg-dim)",
              fontFamily: "monospace", opacity: 0.6,
              overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
              marginBottom: 8,
            }} title={details.path}>
              {details.path}
            </div>
          )}
        </>
      )}

      <div style={{ display: "flex", gap: 6, marginTop: 4 }}>
        <button
          onClick={commit}
          style={{
            flex: 1, padding: "6px 10px", borderRadius: 6, fontSize: 11,
            background: "var(--cyan)", color: "var(--bg-dark)",
            border: "none", cursor: "pointer", fontWeight: 600,
          }}
        >
          Save
        </button>
        <button
          onClick={onClose}
          style={{
            padding: "6px 10px", borderRadius: 6, fontSize: 11,
            background: "var(--bg-surface)", color: "var(--fg-dim)",
            border: "1px solid var(--border)", cursor: "pointer",
          }}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}

function ChainPill({
  step, providers, onUpdate, onRemove,
}: {
  step: ChainStep;
  providers: ProviderOption[];
  onUpdate: (s: ChainStep) => void;
  onRemove: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const colorMap = { prompt: "var(--orange)", agent: "var(--purple)", pipeline: "var(--green)" };
  const iconMap = {
    prompt: <FileText size={9} />,
    agent: <Bot size={9} />,
    pipeline: <Play size={9} />,
  };
  const color = colorMap[step.type];

  // Show executor override badge for prompt steps
  const overrideBadge = step.type === "prompt" && step.executorOverride
    ? `· ${step.executorOverride}`
    : "";

  return (
    <div style={{ position: "relative", display: "inline-block" }}>
      <div
        onClick={() => setEditing(!editing)}
        style={{
          display: "inline-flex", alignItems: "center", gap: 4,
          padding: "4px 9px 4px 7px", borderRadius: 6, fontSize: 11, fontWeight: 500,
          background: color + "18", border: "1px solid " + color + "40",
          color, cursor: "pointer",
        }}
      >
        {iconMap[step.type]}
        <span style={{ maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {step.label}
        </span>
        {overrideBadge && (
          <span style={{ fontSize: 9, opacity: 0.7 }}>{overrideBadge}</span>
        )}
        <button
          onClick={(e) => { e.stopPropagation(); onRemove(); }}
          style={{ background: "none", border: "none", color, cursor: "pointer", padding: 0, display: "flex", opacity: 0.6 }}
        >
          <X size={9} />
        </button>
      </div>
      {editing && (
        <StepEditor
          step={step}
          providers={providers}
          onSave={onUpdate}
          onClose={() => setEditing(false)}
        />
      )}
    </div>
  );
}

export function ChatInput({
  disabled, providers, selectedProvider, selectedModel,
  chain, onSelectProvider, onUpdateChainStep, onRemoveChainStep,
  onClearChain, onSaveWorkflow, onSend,
}: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [savingWorkflow, setSavingWorkflow] = useState(false);
  const [workflowName, setWorkflowName] = useState("");

  const handleSend = useCallback(() => {
    if (!ref.current) return;
    const val = ref.current.value.trim();
    if ((!val && chain.length === 0) || disabled) return;
    onSend(val);
    ref.current.value = "";
    ref.current.style.height = "";
  }, [disabled, onSend, chain.length]);

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

  function commitSaveWorkflow() {
    const name = workflowName.trim();
    if (!name) return;
    onSaveWorkflow(name);
    setWorkflowName("");
    setSavingWorkflow(false);
  }

  const currentProvider = providers.find((p) => p.id === selectedProvider);
  const providerLabel = currentProvider?.label ?? "observer";

  return (
    <div style={{ borderTop: "1px solid var(--border)", background: "var(--bg-dark)", padding: "10px 20px 14px" }}>
      <div style={{ maxWidth: 760, margin: "0 auto" }}>
        {/* Chain builder strip */}
        {chain.length > 0 && (
          <div style={{
            display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap",
            padding: "10px 12px", marginBottom: 10,
            background: "var(--bg-surface)", border: "1px solid var(--border)",
            borderRadius: 10,
          }}>
            {chain.map((step, i) => (
              <div key={i} style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                {i > 0 && <ChevronRight size={11} style={{ color: "var(--fg-dim)", opacity: 0.5 }} />}
                <ChainPill
                  step={step}
                  providers={providers}
                  onUpdate={(s) => onUpdateChainStep(i, s)}
                  onRemove={() => onRemoveChainStep(i)}
                />
              </div>
            ))}

            <div style={{ marginLeft: "auto", display: "flex", gap: 4, alignItems: "center" }}>
              {savingWorkflow ? (
                <>
                  <input
                    autoFocus
                    value={workflowName}
                    onChange={(e) => setWorkflowName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") commitSaveWorkflow();
                      if (e.key === "Escape") { setSavingWorkflow(false); setWorkflowName(""); }
                    }}
                    placeholder="Workflow name..."
                    style={{
                      padding: "3px 8px", fontSize: 11,
                      background: "var(--bg)", border: "1px solid var(--border-bright)",
                      borderRadius: 4, color: "var(--fg)", outline: "none",
                      fontFamily: "inherit", width: 140,
                    }}
                  />
                  <button
                    onClick={commitSaveWorkflow}
                    style={{
                      padding: "3px 8px", fontSize: 10, fontWeight: 600,
                      background: "var(--cyan)", color: "var(--bg-dark)",
                      border: "none", borderRadius: 4, cursor: "pointer",
                    }}
                  >
                    save
                  </button>
                </>
              ) : (
                <button
                  onClick={() => setSavingWorkflow(true)}
                  style={{
                    background: "none", border: "none",
                    color: "var(--cyan)", cursor: "pointer", padding: "3px 6px",
                    borderRadius: 4, display: "flex", alignItems: "center",
                    fontSize: 10, gap: 4, opacity: 0.8,
                  }}
                  title="Save as workflow"
                >
                  <Save size={11} /> save
                </button>
              )}
              <button
                onClick={onClearChain}
                style={{
                  background: "none", border: "none",
                  color: "var(--fg-dim)", cursor: "pointer", padding: "3px 6px",
                  borderRadius: 4, display: "flex", alignItems: "center",
                  fontSize: 10, gap: 3, opacity: 0.5,
                }}
                title="Clear chain"
              >
                <X size={10} /> clear
              </button>
            </div>
          </div>
        )}

        {/* Input area with embedded provider picker */}
        <div style={{
          display: "flex", alignItems: "flex-end", gap: 6,
          background: "var(--bg)", border: "1px solid var(--border)",
          borderRadius: 12, padding: "4px 4px 4px 14px",
        }}>
          <textarea
            ref={ref} rows={1}
            onKeyDown={handleKeyDown} onInput={handleInput}
            disabled={disabled} autoFocus
            placeholder={chain.length > 0 ? "Add context or just send the chain..." : "Ask about your repos, agents, and activity..."}
            style={{
              flex: 1, background: "none", border: "none", color: "var(--fg)",
              fontSize: 13, fontFamily: "-apple-system, system-ui, sans-serif",
              resize: "none", maxHeight: 140, overflowY: "auto", outline: "none",
              lineHeight: 1.5, padding: "6px 0",
            }}
          />

          {/* Provider picker - compact, right of input */}
          <div style={{ position: "relative", display: "inline-block", flexShrink: 0 }}>
            <button
              onClick={() => setPickerOpen(!pickerOpen)}
              style={{
                display: "flex", alignItems: "center", gap: 3,
                padding: "5px 7px", borderRadius: 8, fontSize: 10,
                background: "transparent",
                border: "1px solid var(--border)",
                color: "var(--fg-dim)", cursor: "pointer",
                whiteSpace: "nowrap",
              }}
              title={`${providerLabel} / ${selectedModel || "default"}`}
            >
              <Cpu size={10} style={{ color: "var(--cyan)" }} />
              <span style={{ color: "var(--cyan)" }}>{providerLabel}</span>
              <ChevronDown size={9} />
            </button>

            {pickerOpen && (
              <div style={{
                position: "absolute", bottom: "100%", right: 0, marginBottom: 4,
                background: "var(--bg-dark)", border: "1px solid var(--border-bright)",
                borderRadius: 8, padding: 4, minWidth: 220, maxHeight: 300, overflowY: "auto",
                zIndex: 100, boxShadow: "0 4px 16px rgba(0,0,0,0.4)",
              }}>
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

          {/* Send button */}
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
    </div>
  );
}

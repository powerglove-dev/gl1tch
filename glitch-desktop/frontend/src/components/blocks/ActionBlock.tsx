import { useState } from "react";
import { Loader2, Check, X } from "lucide-react";

interface Props {
  id: string;
  label: string;
  method: string;
  args?: unknown[];
  onAction: (method: string, args?: unknown[]) => Promise<void>;
}

export function ActionBlock({ id, label, method, args, onAction }: Props) {
  const [state, setState] = useState<"idle" | "loading" | "done" | "error">(
    "idle",
  );

  async function handleClick() {
    setState("loading");
    try {
      await onAction(method, args);
      setState("done");
    } catch {
      setState("error");
    }
  }

  return (
    <div className="my-2 flex items-center gap-2">
      <button
        onClick={handleClick}
        disabled={state === "loading" || state === "done"}
        className={`
          inline-flex items-center gap-2 px-4 py-2 rounded-lg text-[13px] font-medium transition-all
          ${state === "done" ? "bg-green/20 text-green border border-green/30" : ""}
          ${state === "error" ? "bg-red/20 text-red border border-red/30" : ""}
          ${state === "idle" ? "bg-purple/20 text-purple border border-purple/30 hover:bg-purple/30 cursor-pointer" : ""}
          ${state === "loading" ? "bg-surface text-fg-muted border border-surface cursor-wait" : ""}
        `}
      >
        {state === "loading" && <Loader2 size={14} className="animate-spin" />}
        {state === "done" && <Check size={14} />}
        {state === "error" && <X size={14} />}
        {label}
      </button>
    </div>
  );
}

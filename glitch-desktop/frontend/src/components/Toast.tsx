/**
 * Reusable toast notifications with action buttons.
 *
 * Mount <ToastProvider> once near the root, then call useToast() from
 * anywhere to push notifications. Toasts stack bottom-right, auto-
 * dismiss after a timeout (paused on hover so the user can read long
 * messages or click an action), and can carry one or more action
 * buttons that fire a callback when clicked.
 *
 * Designed for the kind of feedback the desktop app needs everywhere:
 * "save failed → [retry]", "workflow saved → [open]", "brain offline →
 * [reconnect]". The shape (severity + title + actions) is the same for
 * every call so the UI stays predictable.
 */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { CheckCircle2, AlertTriangle, XCircle, Info, X } from "lucide-react";

export type ToastSeverity = "info" | "success" | "warn" | "error";

/** A single action rendered as a button on the toast. */
export interface ToastAction {
  label: string;
  onClick: () => void;
  /** When true, dismisses the toast after the action runs. Default: true. */
  dismissOnClick?: boolean;
}

export interface ToastInput {
  /** Short, scannable headline. Required. */
  title: string;
  /** Optional second line for detail or context. */
  detail?: string;
  /** Visual severity. Default: "info". */
  severity?: ToastSeverity;
  /** Auto-dismiss in ms. Default: 5000 (info/success) or 8000 (warn/error). 0 = sticky. */
  duration?: number;
  /** Up to ~3 actions. More than that and the toast gets unreadable. */
  actions?: ToastAction[];
}

interface ToastRecord extends ToastInput {
  id: string;
  pausedAt: number | null;
  remaining: number;
}

interface ToastContextValue {
  push: (toast: ToastInput) => string;
  dismiss: (id: string) => void;
  /** Convenience: shorter than push({title, severity: "error"}). */
  error: (title: string, opts?: Omit<ToastInput, "title" | "severity">) => string;
  success: (title: string, opts?: Omit<ToastInput, "title" | "severity">) => string;
}

const ToastContext = createContext<ToastContextValue | null>(null);

/**
 * Hook for emitting toasts. Throws if used outside <ToastProvider> so
 * misconfiguration fails loud at first call instead of silently
 * dropping notifications.
 */
export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used inside <ToastProvider>");
  return ctx;
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastRecord[]>([]);
  // We use refs for the running timers so dismiss/pause can clear them
  // without trampling React state. Keyed by toast id.
  const timersRef = useRef<Map<string, number>>(new Map());

  const dismiss = useCallback((id: string) => {
    const t = timersRef.current.get(id);
    if (t) {
      window.clearTimeout(t);
      timersRef.current.delete(id);
    }
    setToasts((prev) => prev.filter((x) => x.id !== id));
  }, []);

  const startTimer = useCallback(
    (id: string, ms: number) => {
      // Sticky toasts (duration=0) don't get a timer at all.
      if (ms <= 0) return;
      const handle = window.setTimeout(() => dismiss(id), ms);
      timersRef.current.set(id, handle);
    },
    [dismiss],
  );

  const push = useCallback(
    (input: ToastInput): string => {
      const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const sev = input.severity ?? "info";
      // Errors/warnings stick around longer because the user usually
      // needs to read more text or click an action.
      const defaultDuration = sev === "error" || sev === "warn" ? 8000 : 5000;
      const duration = input.duration ?? defaultDuration;
      const record: ToastRecord = {
        id,
        title: input.title,
        detail: input.detail,
        severity: sev,
        duration,
        actions: input.actions,
        pausedAt: null,
        remaining: duration,
      };
      setToasts((prev) => [...prev, record]);
      startTimer(id, duration);
      return id;
    },
    [startTimer],
  );

  const error = useCallback(
    (title: string, opts?: Omit<ToastInput, "title" | "severity">) =>
      push({ title, severity: "error", ...opts }),
    [push],
  );

  const success = useCallback(
    (title: string, opts?: Omit<ToastInput, "title" | "severity">) =>
      push({ title, severity: "success", ...opts }),
    [push],
  );

  // Pause auto-dismiss while the cursor sits on a toast — gives the
  // user time to read long error details without the toast vanishing
  // mid-sentence.
  const pause = useCallback((id: string) => {
    const handle = timersRef.current.get(id);
    if (handle) {
      window.clearTimeout(handle);
      timersRef.current.delete(id);
    }
    setToasts((prev) =>
      prev.map((t) => {
        if (t.id !== id || t.pausedAt != null) return t;
        // remaining = whatever was left when we paused
        return { ...t, pausedAt: Date.now() };
      }),
    );
  }, []);

  const resume = useCallback(
    (id: string) => {
      setToasts((prev) =>
        prev.map((t) => {
          if (t.id !== id || t.pausedAt == null) return t;
          // Don't recompute remaining — we paused at full remaining and
          // never let it tick down. Restart the timer with the same value.
          startTimer(id, t.remaining);
          return { ...t, pausedAt: null };
        }),
      );
    },
    [startTimer],
  );

  // Clean up any orphaned timers on unmount so we don't leak handles
  // if the provider tree gets rebuilt (HMR / route change).
  useEffect(() => {
    return () => {
      for (const handle of timersRef.current.values()) {
        window.clearTimeout(handle);
      }
      timersRef.current.clear();
    };
  }, []);

  return (
    <ToastContext.Provider value={{ push, dismiss, error, success }}>
      {children}
      <ToastViewport
        toasts={toasts}
        onDismiss={dismiss}
        onMouseEnter={pause}
        onMouseLeave={resume}
      />
    </ToastContext.Provider>
  );
}

/**
 * The fixed-position container that renders the active toast stack.
 * Lives in a portal-style fixed div anchored bottom-right so the
 * toasts overlay any modal or popup without their parent's overflow
 * clipping them.
 */
function ToastViewport({
  toasts,
  onDismiss,
  onMouseEnter,
  onMouseLeave,
}: {
  toasts: ToastRecord[];
  onDismiss: (id: string) => void;
  onMouseEnter: (id: string) => void;
  onMouseLeave: (id: string) => void;
}) {
  if (toasts.length === 0) return null;
  return (
    <div
      style={{
        position: "fixed",
        bottom: 20,
        right: 20,
        zIndex: 10000,
        display: "flex",
        flexDirection: "column",
        gap: 8,
        maxWidth: 380,
        pointerEvents: "none", // children re-enable
      }}
      // The viewport itself isn't interactive — only the cards are.
      aria-live="polite"
      aria-atomic="false"
    >
      {toasts.map((t) => (
        <ToastCard
          key={t.id}
          toast={t}
          onDismiss={() => onDismiss(t.id)}
          onMouseEnter={() => onMouseEnter(t.id)}
          onMouseLeave={() => onMouseLeave(t.id)}
        />
      ))}
    </div>
  );
}

function ToastCard({
  toast,
  onDismiss,
  onMouseEnter,
  onMouseLeave,
}: {
  toast: ToastRecord;
  onDismiss: () => void;
  onMouseEnter: () => void;
  onMouseLeave: () => void;
}) {
  const palette = severityPalette(toast.severity ?? "info");
  const Icon = palette.icon;
  return (
    <div
      role="status"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
      style={{
        pointerEvents: "auto",
        background: "var(--bg-dark)",
        border: `1px solid ${palette.border}`,
        borderLeft: `3px solid ${palette.accent}`,
        borderRadius: 8,
        padding: "10px 12px",
        boxShadow: "0 8px 24px rgba(0,0,0,0.45)",
        display: "flex",
        gap: 10,
        alignItems: "flex-start",
        animation: "toast-in 0.18s ease-out",
        fontSize: 12,
      }}
    >
      <Icon size={14} style={{ color: palette.accent, flexShrink: 0, marginTop: 1 }} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div
          style={{
            color: "var(--fg)",
            fontWeight: 600,
            lineHeight: 1.35,
            wordBreak: "break-word",
          }}
        >
          {toast.title}
        </div>
        {toast.detail && (
          <div
            style={{
              marginTop: 3,
              color: "var(--fg-dim)",
              fontSize: 11,
              lineHeight: 1.45,
              wordBreak: "break-word",
            }}
          >
            {toast.detail}
          </div>
        )}
        {toast.actions && toast.actions.length > 0 && (
          <div style={{ marginTop: 8, display: "flex", gap: 6, flexWrap: "wrap" }}>
            {toast.actions.map((a, i) => (
              <button
                key={i}
                onClick={() => {
                  a.onClick();
                  if (a.dismissOnClick !== false) onDismiss();
                }}
                style={{
                  background: "transparent",
                  border: `1px solid ${palette.border}`,
                  color: palette.accent,
                  padding: "3px 10px",
                  borderRadius: 5,
                  cursor: "pointer",
                  fontSize: 11,
                  fontFamily: "inherit",
                  fontWeight: 600,
                }}
              >
                {a.label}
              </button>
            ))}
          </div>
        )}
      </div>
      <button
        onClick={onDismiss}
        title="Dismiss"
        aria-label="Dismiss notification"
        style={{
          background: "none",
          border: "none",
          color: "var(--fg-dim)",
          cursor: "pointer",
          padding: 2,
          display: "flex",
          flexShrink: 0,
          opacity: 0.6,
        }}
      >
        <X size={12} />
      </button>
    </div>
  );
}

/**
 * Maps a severity to its border + accent color and icon. Centralised
 * here so every toast in the app stays visually consistent and we can
 * tweak the palette without hunting through callsites.
 */
function severityPalette(s: ToastSeverity) {
  switch (s) {
    case "success":
      return {
        accent: "var(--green)",
        border: "rgba(158,206,106,0.35)",
        icon: CheckCircle2,
      };
    case "warn":
      return {
        accent: "var(--yellow)",
        border: "rgba(224,175,104,0.35)",
        icon: AlertTriangle,
      };
    case "error":
      return {
        accent: "var(--red)",
        border: "rgba(247,118,142,0.4)",
        icon: XCircle,
      };
    default:
      return {
        accent: "var(--cyan)",
        border: "rgba(125,207,255,0.3)",
        icon: Info,
      };
  }
}

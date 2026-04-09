import { useEffect, useRef } from "react";

interface Props {
  title: string;
  message: string;
  confirmLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmModal({
  title,
  message,
  confirmLabel = "Delete",
  onConfirm,
  onCancel,
}: Props) {
  const cancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    cancelRef.current?.focus();
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onCancel]);

  return (
    <div
      onClick={onCancel}
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 9999,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "rgba(0,0,0,0.6)",
        backdropFilter: "blur(2px)",
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "var(--bg-surface, #1e1f29)",
          border: "1px solid var(--border-bright, #44475a)",
          borderRadius: 10,
          padding: "20px 24px",
          minWidth: 320,
          maxWidth: 420,
          boxShadow: "0 8px 32px rgba(0,0,0,0.5)",
        }}
      >
        <div style={{ fontSize: 14, fontWeight: 600, color: "var(--fg)", marginBottom: 8 }}>
          {title}
        </div>
        <div style={{ fontSize: 12, color: "var(--fg-dim)", lineHeight: 1.5, marginBottom: 20 }}>
          {message}
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button
            ref={cancelRef}
            onClick={onCancel}
            style={{
              padding: "6px 14px",
              fontSize: 12,
              borderRadius: 6,
              border: "1px solid var(--border)",
              background: "transparent",
              color: "var(--fg-dim)",
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            style={{
              padding: "6px 14px",
              fontSize: 12,
              borderRadius: 6,
              border: "1px solid var(--red, #ff5555)",
              background: "rgba(255,85,85,0.15)",
              color: "var(--red, #ff5555)",
              cursor: "pointer",
              fontWeight: 600,
            }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

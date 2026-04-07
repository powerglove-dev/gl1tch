import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ChevronDown, ChevronUp } from "lucide-react";

interface Props {
  children: ReactNode;
  /** Collapsed max-height in pixels. */
  maxHeight?: number;
  /** Label shown on the expand button when there's extra context (e.g. "pasted code"). */
  label?: string;
}

/**
 * Wraps content and clamps it to `maxHeight` when it overflows, revealing a
 * "Show more" toggle. Used to keep large paste blobs from dominating the chat.
 */
export function Collapsible({ children, maxHeight = 220, label }: Props) {
  const innerRef = useRef<HTMLDivElement>(null);
  const [overflowing, setOverflowing] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useLayoutEffect(() => {
    const el = innerRef.current;
    if (!el) return;
    const check = () => setOverflowing(el.scrollHeight > maxHeight + 4);
    check();
    const ro = new ResizeObserver(check);
    ro.observe(el);
    return () => ro.disconnect();
  }, [maxHeight, children]);

  const clamped = overflowing && !expanded;

  return (
    <div style={{ position: "relative" }}>
      <div
        ref={innerRef}
        style={{
          maxHeight: clamped ? maxHeight : "none",
          overflow: "hidden",
          transition: "max-height 120ms ease-out",
        }}
      >
        {children}
      </div>

      {clamped && (
        <div
          style={{
            position: "absolute",
            left: 0,
            right: 0,
            bottom: 28,
            height: 48,
            pointerEvents: "none",
            background:
              "linear-gradient(to bottom, rgba(26,27,38,0) 0%, var(--bg) 100%)",
          }}
        />
      )}

      {overflowing && (
        <button
          onClick={() => setExpanded((v) => !v)}
          style={{
            marginTop: 4,
            display: "inline-flex",
            alignItems: "center",
            gap: 4,
            background: "var(--bg-surface)",
            border: "1px solid var(--border)",
            borderRadius: 6,
            padding: "3px 8px",
            fontSize: 11,
            fontFamily: "inherit",
            color: "var(--fg-dim)",
            cursor: "pointer",
          }}
          onMouseEnter={(e) => (e.currentTarget.style.color = "var(--fg)")}
          onMouseLeave={(e) => (e.currentTarget.style.color = "var(--fg-dim)")}
        >
          {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
          {expanded ? "Show less" : label ? `Show more (${label})` : "Show more"}
        </button>
      )}
    </div>
  );
}

/**
 * Tiny time-formatting helpers shared by the chat surface and the brain
 * activity panel. Centralized so the 12hr / day-separator behavior is
 * identical across views.
 *
 * The user wants:
 *  - 12hr times everywhere (e.g. "10:42 PM")
 *  - Day separators in the chat ("── Today ──", "── Apr 5 ──")
 */

/** Format a unix-millis timestamp as 12hr time, e.g. "10:42 PM". */
export function formatTime12(ts: number): string {
  if (!ts) return "";
  return new Date(ts).toLocaleTimeString(undefined, {
    hour: "numeric",
    minute: "2-digit",
    hour12: true,
  });
}

/** Same as formatTime12 but includes seconds. */
export function formatTime12Seconds(ts: number): string {
  if (!ts) return "";
  return new Date(ts).toLocaleTimeString(undefined, {
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
    hour12: true,
  });
}

/**
 * Returns "Today", "Yesterday", or a short month/day label like
 * "Apr 5, 2026". Used for chat day separators.
 */
export function dayLabel(ts: number): string {
  const d = new Date(ts);
  const now = new Date();
  const startOfDay = (x: Date) =>
    new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const dDay = startOfDay(d);
  const today = startOfDay(now);
  const dayMs = 86_400_000;
  if (dDay === today) return "Today";
  if (dDay === today - dayMs) return "Yesterday";
  const sameYear = d.getFullYear() === now.getFullYear();
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: sameYear ? undefined : "numeric",
  });
}

/**
 * True if two timestamps fall on different calendar days. Used by the
 * chat list to decide whether to insert a day separator before a row.
 */
export function isNewDay(prev: number, next: number): boolean {
  if (!prev || !next) return false;
  const a = new Date(prev);
  const b = new Date(next);
  return (
    a.getFullYear() !== b.getFullYear() ||
    a.getMonth() !== b.getMonth() ||
    a.getDate() !== b.getDate()
  );
}

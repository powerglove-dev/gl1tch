import { useEffect, useRef } from "react";

/** Subscribe to a Wails event, auto-cleanup on unmount. */
export function useWailsEvent<T = unknown>(
  event: string,
  handler: (data: T) => void,
) {
  const handlerRef = useRef(handler);
  handlerRef.current = handler;

  useEffect(() => {
    // Wails runtime is only available inside the WebView, not in plain browser
    const rt = (window as any).runtime;
    if (!rt?.EventsOn) return;

    const cb = (data: T) => handlerRef.current(data);
    rt.EventsOn(event, cb);
    return () => rt.EventsOff(event);
  }, [event]);
}

/**
 * collectorBridge — thin wrapper around the Wails runtime for the
 * structured collector config modal.
 *
 * Why this file exists: glitch-desktop/wailsjs/go/main/App.{d.ts,js}
 * is auto-generated from glitch-desktop/app.go on every `wails build`,
 * so we can't reliably hand-edit it to add the GetCollectorsConfigJSON
 * / WriteCollectorsConfigJSON stubs ahead of a rebuild. Instead this
 * file calls the methods through window.go.main.App directly, the
 * same surface the generated bindings call into. After the next
 * `wails build` the typed bindings will appear and callers could
 * switch to importing them — but going through this bridge keeps the
 * modal independent of the codegen lifecycle.
 */

interface AppBridge {
  GetCollectorsConfigJSON(workspaceId: string): Promise<string>;
  WriteCollectorsConfigJSON(workspaceId: string, jsonContent: string): Promise<string>;
}

function bridge(): AppBridge | null {
  // The Wails runtime injects window.go.main.App at app start. In
  // unit-test contexts (jsdom) the global is missing, so we return
  // null and let the caller decide how to degrade.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const w = window as any;
  if (!w?.go?.main?.App) return null;
  return w.go.main.App as AppBridge;
}

export async function getCollectorsConfigJSON(workspaceId: string): Promise<string> {
  const b = bridge();
  if (!b) throw new Error("Wails runtime not available");
  return b.GetCollectorsConfigJSON(workspaceId);
}

export async function writeCollectorsConfigJSON(
  workspaceId: string,
  jsonContent: string,
): Promise<string> {
  const b = bridge();
  if (!b) throw new Error("Wails runtime not available");
  return b.WriteCollectorsConfigJSON(workspaceId, jsonContent);
}

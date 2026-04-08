import { test, expect } from "@playwright/test";

// smoke.spec.ts — minimal happy-path E2E against the Wails dev server.
//
// Every test in this file runs against the real Go backend via the
// Wails runtime bridge at http://localhost:34115. There is no mock
// layer: `window.go.main.App` resolves to actual Go method calls, so
// assertions about list lengths, streamed chat chunks, and workspace
// state reflect what a user would see in the desktop window.
//
// Test philosophy:
//
//   - Each spec asserts a single user-visible fact. We are not
//     trying to cover every branch here — the Go unit tests and the
//     capability smoke tests already do that. These exist to catch
//     the "it compiles, it serves, but the chat doesn't actually
//     render" regressions that unit tests cannot.
//
//   - Selectors prefer placeholder text and role queries over DOM
//     structure, so refactors of the component tree don't spuriously
//     break the suite. When a stable selector is impossible we add
//     data-testid rather than lock onto a class name.

test.describe("glitch-desktop smoke", () => {
  test("wails bridge exposes the Go backend", async ({ page }) => {
    // The simplest possible check: the wails runtime must inject
    // window.go.main.App with the Ready() method the App component
    // calls during mount. Without this the frontend would render
    // but never transition out of the initial loading state.
    await page.goto("/");
    await page.waitForLoadState("domcontentloaded");
    const hasBridge = await page.evaluate(() => {
      // @ts-expect-error — injected at runtime by the wails runtime.
      return typeof window?.go?.main?.App?.Ready === "function";
    });
    expect(hasBridge).toBe(true);
  });

  test("chat input mounts with a placeholder", async ({ page }) => {
    // Proves the main chat surface renders. ChatInput's placeholder
    // varies by state (`streaming`, `chain.length > 0`, default), so
    // we match the stable suffix that is present in every case.
    await page.goto("/");
    const input = page.getByPlaceholder(/Ask about your repos|Compose next|Add context/);
    await expect(input).toBeVisible({ timeout: 15_000 });
  });

  test("Go backend responds to a bridge call without throwing", async ({ page }) => {
    // ListProviders() may legitimately return null on a fresh install
    // (no providers configured yet) or an array (common case). Either
    // is a valid backend response. What we're actually catching here
    // is the failure mode where the wails bridge is exposed but Go
    // method invocation panics or times out — that's the class of
    // regression this test exists to guard against.
    await page.goto("/");
    // Wait for the chat input to mount, so we know the App component
    // has completed its initial bridge calls (Ready, ListProviders,
    // ListWorkspaces, ...) before we issue our own.
    await page
      .getByPlaceholder(/Ask about your repos|Compose next|Add context/)
      .waitFor({ state: "visible", timeout: 15_000 });

    const { ok, typeName, err } = await page.evaluate(async () => {
      try {
        // @ts-expect-error — wails runtime injection.
        const v = await window.go.main.App.ListProviders();
        const t = v === null
          ? "null"
          : Array.isArray(v)
            ? "array"
            : typeof v;
        return { ok: true, typeName: t, err: null as string | null };
      } catch (e) {
        return { ok: false, typeName: "error", err: String(e) };
      }
    });

    // ListProviders returns []ProviderInfo — which Wails marshals to
    // an array, or to null when the slice is empty. The actual pain
    // point this test catches is a panic/reject, so the type check is
    // generous: we just want the bridge to deliver *something* the
    // frontend could consume.
    expect(err, `ListProviders rejected: ${err}`).toBeNull();
    expect(ok).toBe(true);
    // "undefined" would mean the binding vanished mid-call; any
    // concrete type (array / null / object / string) means the
    // bridge delivered a payload.
    expect(typeName).not.toBe("undefined");
  });
});

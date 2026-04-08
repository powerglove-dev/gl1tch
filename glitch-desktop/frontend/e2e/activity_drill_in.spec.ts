import { test, expect } from "@playwright/test";

// activity_drill_in.spec.ts — smoke coverage for the activity
// sidebar's drill-in modal + ad-hoc analysis pipeline.
//
// These tests do NOT try to produce a real indexing activity event
// (that would require running a collector end-to-end against a live
// ES cluster, which we don't want in a desktop smoke test). Instead
// they verify the Wails bridge contract: the three new bindings
// must be exposed as callable functions on window.go.main.App, and
// ListIndexedDocs must accept the expected argument shape and
// return a parseable JSON response.
//
// The actual UI interaction (clicking an indexing row, selecting
// docs, running Analyze) is exercised by the unit/integration
// tests on the modal component and by hand during release. These
// smoke tests exist to catch the class of regression where a
// binding silently vanishes from the generated wailsjs glue
// because of a typo in the Go signature or a missed Wails rebuild.

test.describe("activity drill-in bridge", () => {
  test("Wails bridge exposes the three new activity bindings", async ({ page }) => {
    // All three bindings are generated into window.go.main.App by
    // the Wails build. If any one is missing the user sees "broken
    // modal — button does nothing" with no visible error; this
    // test fails loudly instead.
    await page.goto("/");
    await page.waitForLoadState("domcontentloaded");

    const bridge = await page.evaluate(() => {
      const w = window as unknown as {
        go?: { main?: { App?: Record<string, unknown> } };
      };
      const app = w.go?.main?.App;
      return {
        hasListIndexedDocs: typeof app?.ListIndexedDocs === "function",
        hasAnalyzeActivityChunks: typeof app?.AnalyzeActivityChunks === "function",
        hasCancelActivityAnalysis: typeof app?.CancelActivityAnalysis === "function",
      };
    });

    expect(bridge.hasListIndexedDocs, "ListIndexedDocs binding missing").toBe(true);
    expect(bridge.hasAnalyzeActivityChunks, "AnalyzeActivityChunks binding missing").toBe(
      true,
    );
    expect(bridge.hasCancelActivityAnalysis, "CancelActivityAnalysis binding missing").toBe(
      true,
    );
  });

  test("ListIndexedDocs returns a parseable JSON string", async ({ page }) => {
    // Call the binding with a source that may or may not exist.
    // On a fresh install ES may not have the index, in which case
    // the backend returns an empty docs array (NOT an error — the
    // 404 path is normalized upstream). If ES is unreachable the
    // backend returns a JSON error object. Either is a valid
    // response; what we're catching here is a panic, a reject, or
    // a bare-string non-JSON return.
    await page.goto("/");
    await page
      .getByPlaceholder(/Ask about your repos|Compose next|Add context/)
      .waitFor({ state: "visible", timeout: 15_000 });

    const result = await page.evaluate(async () => {
      try {
        const w = window as unknown as {
          go: { main: { App: { ListIndexedDocs: (s: string, n: number, l: number) => Promise<string> } } };
        };
        const raw = await w.go.main.App.ListIndexedDocs("git", 0, 10);
        // Must be a string we can JSON.parse.
        const parsed = JSON.parse(raw);
        return {
          ok: true,
          hasDocs: Array.isArray(parsed?.docs),
          hasError: typeof parsed?.error === "string" && parsed.error.length > 0,
          err: null as string | null,
        };
      } catch (e) {
        return { ok: false, hasDocs: false, hasError: false, err: String(e) };
      }
    });

    expect(result.err, `ListIndexedDocs rejected: ${result.err}`).toBeNull();
    expect(result.ok).toBe(true);
    // Either a docs array (happy path — may be empty) or an error
    // field (ES unreachable on a fresh install) is a valid
    // parse. What's invalid is neither, which would mean the JSON
    // shape regressed.
    expect(result.hasDocs || result.hasError).toBe(true);
  });

  test("AnalyzeActivityChunks rejects malformed request JSON cleanly", async ({ page }) => {
    // The binding's first line of defense is JSON.Unmarshal. A
    // malformed request should return a JSON error object, NOT
    // panic the Go side or return a bare string. This locks the
    // contract for the frontend error-handling path.
    await page.goto("/");
    await page
      .getByPlaceholder(/Ask about your repos|Compose next|Add context/)
      .waitFor({ state: "visible", timeout: 15_000 });

    const result = await page.evaluate(async () => {
      try {
        const w = window as unknown as {
          go: { main: { App: { AnalyzeActivityChunks: (req: string) => Promise<string> } } };
        };
        const raw = await w.go.main.App.AnalyzeActivityChunks("not json at all");
        const parsed = JSON.parse(raw);
        return {
          ok: true,
          hasError: typeof parsed?.error === "string" && parsed.error.length > 0,
          err: null as string | null,
        };
      } catch (e) {
        return { ok: false, hasError: false, err: String(e) };
      }
    });

    expect(result.err, `AnalyzeActivityChunks rejected: ${result.err}`).toBeNull();
    expect(result.ok).toBe(true);
    expect(result.hasError).toBe(true);
  });
});

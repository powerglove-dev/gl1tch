import { defineConfig, devices } from "@playwright/test";

// Playwright E2E against the Wails dev HTTP server.
//
// When `wails dev` runs it starts the Vite dev server behind the Wails
// runtime bridge on http://localhost:34115 by default. Unlike plain
// `vite`, the bridge exposes the Go backend at `window.go.main.App`,
// so the frontend's `wailsjs/go/main/App` imports (RunChain, AskScoped,
// ListWorkspaces, ...) resolve to real Go calls. That means any E2E
// driven through this config exercises the entire stack: browser →
// Vite → Wails runtime → Go → SQLite/ES/Ollama. The tests do not need
// a native window; the dev URL is enough.
//
// How to run:
//   cd glitch-desktop/frontend
//   npm run e2e             # launches wails dev + runs tests headless
//   npm run e2e:headed      # same but with a visible browser window
//   npm run e2e:ui          # Playwright's interactive UI mode
//
// Prerequisites:
//   - `wails` on PATH (we resolve ~/go/bin/wails explicitly below so
//     the test suite works even when $PATH hasn't been refreshed).
//   - A built glitch-desktop Go module (wails dev builds on demand).
//
// CI note: wails dev takes ~10–15s to first-serve the bridge because
// it has to compile the Go app. The `webServer.timeout` below is
// sized accordingly. The `reuseExistingServer` flag lets a developer
// keep `wails dev` running in a side terminal and re-run tests without
// cold-starting every time.
const WAILS_DEV_URL = "http://localhost:34115";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false, // one Wails dev server — run tests serially
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? "line" : "list",
  timeout: 60_000,
  expect: {
    timeout: 10_000,
  },
  use: {
    baseURL: WAILS_DEV_URL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  // Boot `wails dev` before the first test, tear it down when the
  // suite finishes. We run it from the glitch-desktop directory (one
  // level up from frontend) because that is where wails.json lives.
  webServer: {
    command: "../../glitch-desktop/scripts/wails-dev-for-e2e.sh",
    url: WAILS_DEV_URL,
    timeout: 180_000,
    reuseExistingServer: !process.env.CI,
    stdout: "pipe",
    stderr: "pipe",
  },
});

# glitch-desktop E2E tests

Playwright tests that drive the Wails dev HTTP server exactly as a
real user would — through the browser bridge, against the real Go
backend, no mocks.

## Running

```sh
cd glitch-desktop/frontend
npm run e2e             # headless
npm run e2e:headed      # with a visible Chromium window
npm run e2e:ui          # Playwright interactive UI mode
```

The first run takes ~15 seconds because Playwright has to start
`wails dev` (which compiles the Go app) before the first test can
navigate to `http://localhost:34115`. Subsequent runs reuse the
server if it is still running, so you can leave `wails dev -browser`
open in a side terminal and iterate quickly.

## How it works

Playwright's `webServer` hook in `playwright.config.ts` runs
`../../glitch-desktop/scripts/wails-dev-for-e2e.sh`, which invokes
`wails dev -browser`. The `-browser` flag tells Wails to skip the
native window and serve the Vite dev server + runtime bridge over
HTTP instead — so Playwright can drive the frontend with a normal
Chromium context and every call to `window.go.main.App.*` resolves
to a real Go method.

That means these tests exercise the whole stack:

    Playwright → Chromium → Vite → Wails runtime → Go → SQLite/ES/Ollama

No stubs, no shims. If the security_alerts capability doesn't reach
Elasticsearch, these tests will catch it.

## Writing new tests

Put specs in `e2e/*.spec.ts`. Prefer placeholder text and ARIA role
queries to locate elements, and add `data-testid` only when there's
no stable alternative — that way refactors of the React tree don't
spuriously break the suite.

When a test needs to call a Go method directly (bypassing the UI),
use `page.evaluate` and reach through the wails runtime:

```ts
const providers = await page.evaluate(async () =>
  // @ts-expect-error — wails runtime injection
  await window.go.main.App.ListProviders()
);
```

Keep the suite short. The Go unit tests + `internal/capability`
smoke tests are where coverage lives; these E2E tests exist to
catch the "it built but the chat doesn't render" class of bug the
unit tests cannot.

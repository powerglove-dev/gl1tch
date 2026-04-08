#!/usr/bin/env bash
# wails-dev-for-e2e.sh — launch `wails dev` in a mode suitable for
# driving via Playwright.
#
# Playwright's webServer hook runs this script, waits for the Wails
# dev HTTP server at http://localhost:34115 to answer, then starts
# the test suite. The script resolves the wails binary explicitly
# (so $PATH quirks in a Playwright subprocess don't break it), cd's
# into the glitch-desktop module, and launches `wails dev -browser`
# which forwards the Vite dev server + runtime bridge over HTTP
# without popping a native window.
#
# Why a wrapper script rather than inlining the command:
#   1. Playwright's `command` string doesn't run through a shell
#      that inherits the user's login env, so `wails` may not be on
#      PATH even when it works in an interactive terminal.
#   2. Wails subcommands error out if cwd is wrong, and the config
#      file (wails.json) lives at the glitch-desktop root — not the
#      frontend root where Playwright launches from.
#   3. We want one place to toggle extra dev flags (e.g. -loglevel)
#      without editing playwright.config.ts.

set -euo pipefail

# Resolve wails binary: prefer PATH, fall back to ~/go/bin.
WAILS_BIN="$(command -v wails || true)"
if [[ -z "${WAILS_BIN}" ]]; then
  if [[ -x "${HOME}/go/bin/wails" ]]; then
    WAILS_BIN="${HOME}/go/bin/wails"
  else
    echo "wails-dev-for-e2e: wails binary not found on PATH or in ~/go/bin" >&2
    exit 1
  fi
fi

# Locate the glitch-desktop module (one level up from scripts/).
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
DESKTOP_DIR="$( cd "${SCRIPT_DIR}/.." && pwd )"

cd "${DESKTOP_DIR}"

# -browser: skip the native window; serve the Wails runtime bridge
#           over the Vite HTTP port so Playwright can drive it as a
#           regular web page.
# -loglevel warning: quiet the chatty build output so Playwright's
#           webServer capture stays readable.
exec "${WAILS_BIN}" dev -browser -loglevel warning

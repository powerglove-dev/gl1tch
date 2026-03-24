#!/usr/bin/env bash
# Opens a split tmux session for debugging orcai with Delve.
# Left pane: dlv headless server. Right pane: dlv connect client.
set -e

BINARY="bin/orcai-debug"
PORT=2345
SESSION="orcai-dbg"

# Kill any leftover debug session
tmux kill-session -t "$SESSION" 2>/dev/null || true

# Kill anything already on the port
lsof -ti:"$PORT" | xargs kill -9 2>/dev/null || true

echo "Starting debug session '$SESSION' on :$PORT ..."

tmux new-session -d -s "$SESSION" -x 220 -y 50

# Left pane: dlv headless
tmux send-keys -t "$SESSION" \
  "dlv exec --headless --listen=:$PORT --api-version=2 ./$BINARY; echo '--- dlv exited ---'; read" \
  Enter

# Right pane: dlv connect (waits briefly for server to be ready)
tmux split-window -h -t "$SESSION"
tmux send-keys -t "$SESSION" \
  "sleep 0.8 && dlv connect :$PORT" \
  Enter

tmux attach-session -t "$SESSION"

PLUGINS_DIR := ../glitch-plugins

.PHONY: build run plugins debug debug-tmux

build:
	go build -o glitch .

run: build
	./glitch

# Build and install all core provider plugins from ../glitch-plugins.
# Run this during development whenever plugin code changes.
plugins:
	$(MAKE) -C $(PLUGINS_DIR) install

debug:
	dlv debug . -- server

debug-tmux:
	tmux new-session -d -s glitch-debug 2>/dev/null || true
	tmux send-keys -t glitch-debug "dlv debug . -- server" Enter

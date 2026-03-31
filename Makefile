PLUGINS_DIR := ../orcai-plugins

.PHONY: build run plugins debug debug-tmux

build:
	go build -o orcai .

run: build
	./orcai

# Build and install all core provider plugins from ../orcai-plugins.
# Run this during development whenever plugin code changes.
plugins:
	$(MAKE) -C $(PLUGINS_DIR) install

debug:
	dlv debug . -- server

debug-tmux:
	tmux new-session -d -s orcai-debug 2>/dev/null || true
	tmux send-keys -t orcai-debug "dlv debug . -- server" Enter

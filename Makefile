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

.PHONY: test-integration test-integration-astro

# Run all integration tests (requires ollama running with at least llama3.2).
# Override model with: GLITCH_SMOKE_MODEL=llama3.2:1b make test-integration
test-integration:
	go test -tags=integration -timeout=15m -v ./internal/pipeline/...

# Run only the Astro pipeline generation + build test.
# Requires: ollama (llama3.2 or GLITCH_SMOKE_MODEL), bun, shell sidecar.
test-integration-astro:
	go test -tags=integration -timeout=15m -v -run TestAstroPipelineGenAndRun ./internal/pipeline/...

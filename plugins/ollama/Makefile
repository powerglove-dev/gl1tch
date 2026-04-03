BINARY    := gl1tch-ollama
SIDECAR   := ollama.yaml
INSTALL_BIN := $(or $(shell test -w /usr/local/bin && echo /usr/local/bin),$(HOME)/.local/bin)
WRAPPERS_DIR := $(HOME)/.config/glitch/wrappers

.PHONY: build install test clean

build:
	go build -o $(BINARY) .

install: build
	@mkdir -p $(WRAPPERS_DIR)
	install -m 0755 $(BINARY) $(INSTALL_BIN)/$(BINARY)
	install -m 0644 $(SIDECAR) $(WRAPPERS_DIR)/$(SIDECAR)
	@echo "Installed $(BINARY) → $(INSTALL_BIN)/$(BINARY)"
	@echo "Installed $(SIDECAR) → $(WRAPPERS_DIR)/$(SIDECAR)"

test:
	go test ./...

clean:
	rm -f $(BINARY)

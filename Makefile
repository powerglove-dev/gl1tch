BINARY := orcai

.PHONY: build run test clean install debug-build debug debug-connect debug-tmux

all: build run

run: install
	-tmux detach-client -s orcai 2>/dev/null
	-tmux kill-session -t orcai 2>/dev/null
	-tmux kill-session -t orcai-cron 2>/dev/null
	rm -f ~/.config/orcai/layout.yaml ~/.config/orcai/keybindings.yaml
	$(BINARY)

build:
	go build -o bin/$(BINARY) .

install: build
	go install .
	@if [ ! -L ~/.local/bin/$(BINARY) ]; then \
		rm -f ~/.local/bin/$(BINARY); \
		ln -sf $$(go env GOPATH)/bin/$(BINARY) ~/.local/bin/$(BINARY); \
	fi

test:
	go test ./...

clean:
	rm -f bin/$(BINARY) bin/$(BINARY)-debug

debug-build:
	go build -gcflags="all=-N -l" -o bin/$(BINARY)-debug .

debug: debug-build
	@echo "Delve listening on :2345 — connect with: make debug-connect"
	dlv exec --headless --listen=:2345 --api-version=2 ./bin/$(BINARY)-debug

debug-connect:
	dlv connect :2345

debug-tmux: debug-build
	@bash $(shell pwd)/scripts/debug-tmux.sh

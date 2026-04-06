BINARY   = .build/release/GlitchNotify
APP_DIR  = $(HOME)/.local/Applications/glitch-notify.app
BIN_DIR  = $(HOME)/.local/bin

.PHONY: build bundle install run clean

build:
	swift build -c release

bundle: build
	@mkdir -p "$(APP_DIR)/Contents/MacOS" "$(APP_DIR)/Contents/Resources"
	@cp $(BINARY) "$(APP_DIR)/Contents/MacOS/glitch-notify"
	@cp Info.plist "$(APP_DIR)/Contents/"
	@cp assets/icon.png "$(APP_DIR)/Contents/Resources/" 2>/dev/null || true
	@echo "bundled → $(APP_DIR)"

install: bundle
	@mkdir -p "$(BIN_DIR)"
	@ln -sf "$(APP_DIR)/Contents/MacOS/glitch-notify" "$(BIN_DIR)/glitch-notify"
	@echo "installed → $(BIN_DIR)/glitch-notify"

run: bundle
	@open "$(APP_DIR)"

clean:
	swift package clean
	rm -rf "$(APP_DIR)"
	rm -f "$(BIN_DIR)/glitch-notify"

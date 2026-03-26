## 1. Extend SidecarSchema with models field (orcai repo)

- [x] 1.1 Add `SidecarModel struct { ID string \`yaml:"id"\`; Label string \`yaml:"label"\` }` to `internal/plugin/cli_adapter.go`
- [x] 1.2 Add `Models []SidecarModel \`yaml:"models"\`` to `SidecarSchema`
- [x] 1.3 Add `Models() []SidecarModel` accessor to `CliAdapter`; populate it in `NewCliAdapterFromSidecar`
- [x] 1.4 Run `go build ./internal/plugin/...` — zero errors

## 2. Update buildProviders to read sidecar models (orcai repo)

- [x] 2.1 In `buildProviders` (`internal/picker/picker.go`), when appending a `TypeCLIWrapper` extra, locate the sidecar path via `discovery.PluginInfo.Path` and call `plugin.NewCliAdapterFromSidecar` to read its models
- [x] 2.2 Convert `[]plugin.SidecarModel` → `[]picker.ModelOption` and set on the `ProviderDef`
- [x] 2.3 Remove the hardcoded `if name == "opencode" { p = injectOllamaModels(...) }` special-case
- [x] 2.4 Verify `discovery.PluginInfo` exposes `Path` (the sidecar file path); add it if missing
- [x] 2.5 Run `go build ./...` — zero errors

## 3. Remove static provider list (orcai repo)

- [x] 3.1 Remove `claude` and `copilot` entries from the `Providers` slice in `internal/picker/picker.go`; keep only `ollama` and `shell`
- [x] 3.2 Remove `"claude": {"--print"}` from `pipelineLaunchArgs` (moves to sidecar `args`)
- [x] 3.3 Remove the `nativeExtras` / `extras` distinction — all non-static discovered plugins use the same sidecar-model path now
- [x] 3.4 Update `picker_test.go`: remove `claude` and `copilot` from the expected-providers list; add a test that `BuildProviders` returns models from a sidecar fixture
- [x] 3.5 Run `go test ./internal/picker/...` — all pass

## 4. claude plugin (orcai-plugins repo)

- [x] 4.1 Create `plugins/claude/main.go`: reads stdin → `claude --print [--model $ORCAI_MODEL]` → stdout; handles missing `claude` binary with a clear error message
- [x] 4.2 Create `plugins/claude/claude.yaml` sidecar declaring `name: claude`, `command: orcai-claude`, `args: [--print]`, and `models: [{id: claude-opus-4-6, label: "Opus 4.6"}, {id: claude-sonnet-4-6, label: "Sonnet 4.6"}, {id: claude-haiku-4-5-20251001, label: "Haiku 4.5"}]`
- [x] 4.3 Add `claude` to the root `Makefile` build/install targets
- [x] 4.4 Install sidecar to `~/.config/orcai/wrappers/claude.yaml` and binary to `~/.local/bin/orcai-claude`
- [ ] 4.5 Verify: `orcai sysop` → agent runner shows Claude with three models

## 5. github-copilot plugin (orcai-plugins repo)

- [x] 5.1 Create `plugins/github-copilot/main.go`: reads stdin → `gh copilot suggest -t shell` (non-interactive) → stdout; exits cleanly when gh is not installed
- [x] 5.2 Create `plugins/github-copilot/github-copilot.yaml` sidecar declaring `name: github-copilot`, `command: orcai-github-copilot`; `models` list empty (Copilot doesn't expose model selection)
- [x] 5.3 Add `github-copilot` to root `Makefile` build/install targets
- [x] 5.4 Install sidecar to `~/.config/orcai/wrappers/github-copilot.yaml`
- [ ] 5.5 Verify: agent runner shows GitHub Copilot; selecting it skips model step and goes straight to prompt

## 6. gemini plugin (orcai-plugins repo)

- [x] 6.1 Create `plugins/gemini/main.go`: reads stdin → `gemini [--model $ORCAI_MODEL]` → stdout
- [x] 6.2 Create `plugins/gemini/gemini.yaml` sidecar declaring `name: gemini`, `command: orcai-gemini`, and `models: [{id: gemini-2.0-flash, label: "Gemini 2.0 Flash"}, {id: gemini-1.5-pro, label: "Gemini 1.5 Pro"}]`
- [x] 6.3 Add `gemini` to root `Makefile` build/install targets
- [x] 6.4 Install sidecar to `~/.config/orcai/wrappers/gemini.yaml`
- [ ] 6.5 Verify: agent runner shows Gemini with two models

## 7. Update existing plugin sidecars to declare models (orcai-plugins repo)

- [x] 7.1 Add a `models` block to `plugins/opencode/opencode.yaml` listing the user's ollama models (document that users should customise this list); update `~/.config/orcai/wrappers/opencode.yaml`
- [x] 7.2 Add a `models` block to `plugins/ollama/ollama.yaml` — since ollama models are discovered dynamically, document in comments that the list is a default fallback; the sidecar should list `llama3.2:latest` and `qwen2.5:latest` as defaults; update `~/.config/orcai/wrappers/ollama.yaml`
- [x] 7.3 Update `buildProviders` to SKIP the `queryOllamaModels()` runtime injection for ollama if the sidecar already declares models (sidecar takes precedence); only fall back to runtime query if sidecar `models` list is empty
- [ ] 7.4 Verify: agent runner shows opencode and ollama with models from their sidecars

## 8. Push and validate

- [ ] 8.1 `git push` orcai repo changes
- [ ] 8.2 `git push` orcai-plugins repo changes
- [x] 8.3 Run `go test ./...` in orcai repo — all pass
- [ ] 8.4 Run `orcai` from scratch (kill existing session): banner appears, switchboard opens, agent runner lists providers with correct models

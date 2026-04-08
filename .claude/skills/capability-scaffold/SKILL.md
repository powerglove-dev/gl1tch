---
name: capability-scaffold
description: Scaffold a new internal/capability/builtin_<name>.go with the unified Capability interface, registry wiring, Router entry, and a table-driven *_test.go. Use when adding a data source to gl1tch post-collector-unification.
disable-model-invocation: true
---

Scaffold a new capability for gl1tch's unified capability runtime.

Background: internal/collector is gone. Every data source lives in internal/capability and implements the Capability interface defined in internal/capability/capability.go. There is one Runner and one PodManager. The canonical reference is internal/capability/builtin_workspace.go.

When invoked with a name argument (e.g. `/capability-scaffold linear`):

1. Read the contract first — do not guess at signatures:
   - internal/capability/capability.go (Capability interface)
   - internal/capability/registry.go (registration call)
   - internal/capability/router.go (router selection table)
   - internal/capability/builtin_workspace.go (reference impl)
   - internal/capability/pod_manager.go (if the capability needs subprocess execution)

2. Create internal/capability/builtin_<name>.go containing:
   - A struct type `<Name>Capability` with private fields
   - A constructor `New<Name>Capability(...)` returning the Capability interface
   - All methods required by the Capability interface — names, signatures, and return types matching the source of truth, not memory
   - Events emitted via the shared event_sink, not a local logger
   - Any LLM call defaulted to `qwen2.5:7b` via local Ollama (per project_local_llm_default.md)
   - PodManager used for any subprocess work — no bare exec.Command

3. Wire registration:
   - Add the registration call in registry.go (or wherever existing builtins register)
   - Add the routing entry in router.go so `glitch assistant` can select it by name
   - Name string must match the `builtin_<name>.go` filename

4. Create internal/capability/builtin_<name>_test.go using the table-driven style from .claude/skills/golang-testing/SKILL.md:
   - One TestXxx with a `tests := []struct{name string; ...}{...}`
   - t.Run(tc.name, func(t *testing.T){...})
   - At least: happy path, empty input, error path

5. Run `go vet ./internal/capability/...` and `go test ./internal/capability/...` and report results.

Hard rules:
- Do not recreate anything under internal/collector — that path is deleted forever
- Do not add migration code, schema repair, or fallbacks (pre-1.0 rule)
- Do not invent interface methods — read capability.go and use exactly what's there
- Do not add comments narrating the code; only comment non-obvious logic
- Use `glitch-<name>` for any binary name and `gl1tch-<name>` for any plugin repo name (per feedback_naming_convention.md)

If any of the canonical source files have changed shape since this skill was written, trust the source files and adapt — do not force the old shape.

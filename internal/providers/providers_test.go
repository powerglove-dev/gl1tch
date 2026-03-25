package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adam-stokes/orcai/internal/providers"
)

// bundledNames is the expected set of names shipped with the binary.
var bundledNames = []string{"claude", "gemini", "opencode", "aider", "goose", "copilot"}

// ── LoadBundled ──────────────────────────────────────────────────────────────

func TestLoadBundled_ReturnsAllSixProfiles(t *testing.T) {
	profiles, err := providers.LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() unexpected error: %v", err)
	}
	if got, want := len(profiles), len(bundledNames); got != want {
		t.Fatalf("LoadBundled() returned %d profiles, want %d", got, want)
	}
}

func TestLoadBundled_ProfileNames(t *testing.T) {
	profiles, err := providers.LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() unexpected error: %v", err)
	}

	nameSet := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		nameSet[p.Name] = true
	}

	for _, want := range bundledNames {
		if !nameSet[want] {
			t.Errorf("LoadBundled(): missing profile %q", want)
		}
	}
}

func TestLoadBundled_ProfilesHaveBinaryAndModels(t *testing.T) {
	profiles, err := providers.LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() unexpected error: %v", err)
	}

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			if p.Binary == "" {
				t.Errorf("profile %q has empty Binary", p.Name)
			}
			if len(p.Models) == 0 {
				t.Errorf("profile %q has no models", p.Name)
			}
			for _, m := range p.Models {
				if m.ID == "" {
					t.Errorf("profile %q: model with empty ID", p.Name)
				}
			}
		})
	}
}

// ── LoadUser ─────────────────────────────────────────────────────────────────

func TestLoadUser_NonExistentDir(t *testing.T) {
	profiles, err := providers.LoadUser("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("LoadUser() non-existent dir: unexpected error: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("LoadUser() non-existent dir: expected empty slice, got %d profiles", len(profiles))
	}
}

func TestLoadUser_CustomProfile(t *testing.T) {
	dir := t.TempDir()
	yaml := `
name: myai
binary: myai-cli
display_name: My AI
api_key_env: MYAI_API_KEY
models:
  - id: myai-v1
    display: "My AI v1"
    cost_input_per_1m: 1.00
    cost_output_per_1m: 2.00
session:
  window_name: "myai:{{.model}}"
  launch_args: ["--verbose"]
  env:
    MYAI_EXTRA: "1"
`
	if err := os.WriteFile(filepath.Join(dir, "myai.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	profiles, err := providers.LoadUser(dir)
	if err != nil {
		t.Fatalf("LoadUser() unexpected error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("LoadUser(): expected 1 profile, got %d", len(profiles))
	}

	p := profiles[0]
	if p.Name != "myai" {
		t.Errorf("Name: got %q, want %q", p.Name, "myai")
	}
	if p.Binary != "myai-cli" {
		t.Errorf("Binary: got %q, want %q", p.Binary, "myai-cli")
	}
	if p.DisplayName != "My AI" {
		t.Errorf("DisplayName: got %q, want %q", p.DisplayName, "My AI")
	}
	if p.APIKeyEnv != "MYAI_API_KEY" {
		t.Errorf("APIKeyEnv: got %q, want %q", p.APIKeyEnv, "MYAI_API_KEY")
	}
	if len(p.Models) != 1 || p.Models[0].ID != "myai-v1" {
		t.Errorf("Models: unexpected value %+v", p.Models)
	}
	if p.Session.WindowName != "myai:{{.model}}" {
		t.Errorf("Session.WindowName: got %q", p.Session.WindowName)
	}
	if len(p.Session.LaunchArgs) != 1 || p.Session.LaunchArgs[0] != "--verbose" {
		t.Errorf("Session.LaunchArgs: got %v", p.Session.LaunchArgs)
	}
	if p.Session.Env["MYAI_EXTRA"] != "1" {
		t.Errorf("Session.Env[MYAI_EXTRA]: got %q", p.Session.Env["MYAI_EXTRA"])
	}
}

func TestLoadUser_IgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()
	// Write a non-YAML file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	profiles, err := providers.LoadUser(dir)
	if err != nil {
		t.Fatalf("LoadUser() unexpected error: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("LoadUser(): expected 0 profiles, got %d", len(profiles))
	}
}

// ── Registry ─────────────────────────────────────────────────────────────────

func TestRegistry_All_ContainsBundled(t *testing.T) {
	reg, err := providers.NewRegistry("/nonexistent")
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	all := reg.All()
	if len(all) < len(bundledNames) {
		t.Fatalf("Registry.All(): got %d profiles, want at least %d", len(all), len(bundledNames))
	}
}

func TestRegistry_Get(t *testing.T) {
	reg, err := providers.NewRegistry("/nonexistent")
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	p, ok := reg.Get("claude")
	if !ok {
		t.Fatal("Registry.Get(\"claude\"): not found")
	}
	if p.Name != "claude" {
		t.Errorf("Registry.Get(\"claude\"): Name = %q", p.Name)
	}

	_, ok = reg.Get("does-not-exist")
	if ok {
		t.Error("Registry.Get(\"does-not-exist\"): expected false, got true")
	}
}

func TestRegistry_UserOverridesBundled(t *testing.T) {
	dir := t.TempDir()
	// Override the bundled "claude" profile with a custom binary name.
	yaml := `
name: claude
binary: my-custom-claude
display_name: Custom Claude
api_key_env: CUSTOM_KEY
models:
  - id: custom-model
    display: "Custom"
    cost_input_per_1m: 0.00
    cost_output_per_1m: 0.00
session:
  window_name: "claude:custom"
  launch_args: []
  env: {}
`
	if err := os.WriteFile(filepath.Join(dir, "claude.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	reg, err := providers.NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	p, ok := reg.Get("claude")
	if !ok {
		t.Fatal("Registry.Get(\"claude\"): not found after user override")
	}
	if p.Binary != "my-custom-claude" {
		t.Errorf("user profile did not win: Binary = %q, want %q", p.Binary, "my-custom-claude")
	}
	if p.DisplayName != "Custom Claude" {
		t.Errorf("user profile did not win: DisplayName = %q", p.DisplayName)
	}

	// Total count should not grow (override, not append).
	all := reg.All()
	nameCount := make(map[string]int)
	for _, pr := range all {
		nameCount[pr.Name]++
	}
	if nameCount["claude"] != 1 {
		t.Errorf("Registry.All(): %d entries for \"claude\", want 1", nameCount["claude"])
	}
}

func TestRegistry_UserAddsNewProfile(t *testing.T) {
	dir := t.TempDir()
	yaml := `
name: newprovider
binary: newprovider-cli
display_name: New Provider
api_key_env: NEW_KEY
models:
  - id: new-v1
    display: "New v1"
    cost_input_per_1m: 0.00
    cost_output_per_1m: 0.00
session:
  window_name: "new:{{.model}}"
  launch_args: []
  env: {}
`
	if err := os.WriteFile(filepath.Join(dir, "newprovider.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	reg, err := providers.NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	p, ok := reg.Get("newprovider")
	if !ok {
		t.Fatal("Registry.Get(\"newprovider\"): not found")
	}
	if p.Binary != "newprovider-cli" {
		t.Errorf("Binary: got %q", p.Binary)
	}
}

// TestRegistry_Available_RealBinary checks that at least one well-known binary
// (sh or echo) is reported as available by Registry.Available().
func TestRegistry_Available_RealBinary(t *testing.T) {
	dir := t.TempDir()
	// Add a profile whose binary is guaranteed to exist on any POSIX host.
	yaml := `
name: shell
binary: sh
display_name: Shell (test)
api_key_env: ""
models:
  - id: sh-v1
    display: "sh"
    cost_input_per_1m: 0.00
    cost_output_per_1m: 0.00
session:
  window_name: "sh:{{.model}}"
  launch_args: []
  env: {}
`
	if err := os.WriteFile(filepath.Join(dir, "shell.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	reg, err := providers.NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	avail := reg.Available()
	found := false
	for _, p := range avail {
		if p.Name == "shell" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Registry.Available(): expected \"shell\" (binary=sh) to be available")
	}
}

// TestRegistry_Available_MissingBinary ensures that a profile with a
// nonexistent binary does NOT appear in Available().
func TestRegistry_Available_MissingBinary(t *testing.T) {
	dir := t.TempDir()
	yaml := `
name: ghost
binary: orcai-nonexistent-binary-xyz
display_name: Ghost Provider
api_key_env: ""
models:
  - id: ghost-v1
    display: "Ghost"
    cost_input_per_1m: 0.00
    cost_output_per_1m: 0.00
session:
  window_name: "ghost:{{.model}}"
  launch_args: []
  env: {}
`
	if err := os.WriteFile(filepath.Join(dir, "ghost.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	reg, err := providers.NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	for _, p := range reg.Available() {
		if p.Name == "ghost" {
			t.Error("Registry.Available(): \"ghost\" with missing binary should NOT be available")
		}
	}
}

func TestLoadUser_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(": invalid: yaml: ["), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := providers.LoadUser(dir)
	if err == nil {
		t.Fatal("LoadUser() expected error for malformed YAML, got nil")
	}
}

// TestAiderEnvPassThrough checks that the aider profile carries an empty-string
// ANTHROPIC_API_KEY env value (pass-through convention).
func TestAiderEnvPassThrough(t *testing.T) {
	profiles, err := providers.LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() unexpected error: %v", err)
	}

	for _, p := range profiles {
		if p.Name != "aider" {
			continue
		}
		val, ok := p.Session.Env["ANTHROPIC_API_KEY"]
		if !ok {
			t.Fatal("aider profile: Session.Env missing ANTHROPIC_API_KEY key")
		}
		if val != "" {
			t.Errorf("aider profile: ANTHROPIC_API_KEY env value = %q, want empty string (pass-through)", val)
		}
		return
	}
	t.Fatal("aider profile not found in bundled profiles")
}

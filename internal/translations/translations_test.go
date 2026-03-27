package translations_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adam-stokes/orcai/internal/translations"
)

// writeYAML writes content to a temporary translations.yaml and returns the
// full path to the file.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "translations.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return path
}

func TestYAMLProvider_KnownKey(t *testing.T) {
	path := writeYAML(t, "pipelines_panel_title: \"MY PIPES\"\n")
	p := translations.NewYAMLProviderFromPath(path)

	got := p.T(translations.KeyPipelinesTitle, "PIPELINES")
	if got != "MY PIPES" {
		t.Errorf("T(%q) = %q, want %q", translations.KeyPipelinesTitle, got, "MY PIPES")
	}
}

func TestYAMLProvider_UnknownKey(t *testing.T) {
	path := writeYAML(t, "some_other_key: \"value\"\n")
	p := translations.NewYAMLProviderFromPath(path)

	got := p.T("nonexistent_key", "FALLBACK")
	if got != "FALLBACK" {
		t.Errorf("T(%q) = %q, want %q", "nonexistent_key", got, "FALLBACK")
	}
}

func TestYAMLProvider_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does_not_exist.yaml")

	p := translations.NewYAMLProviderFromPath(path)
	if p == nil {
		t.Fatal("NewYAMLProviderFromPath returned nil for missing file, want a valid Provider")
	}
	// Missing file → NopProvider behavior: always return fallback.
	got := p.T("any_key", "DEFAULT")
	if got != "DEFAULT" {
		t.Errorf("T() on missing-file provider = %q, want %q", got, "DEFAULT")
	}
}

func TestExpandEscapes_AnsiExpansion(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`\e[31m`, "\x1b[31m"},
		{`\033[0m`, "\x1b[0m"},
		{`\x1b[32m`, "\x1b[32m"},
		{`\e[1m\e[31m`, "\x1b[1m\x1b[31m"},
		{"no escapes", "no escapes"},
	}

	// We test expandEscapes indirectly via NewYAMLProviderFromPath because the
	// function is unexported.
	for _, tc := range cases {
		path := writeYAML(t, "test_key: '"+tc.input+"'\n")
		p := translations.NewYAMLProviderFromPath(path)
		got := p.T("test_key", "")
		if got != tc.want {
			t.Errorf("expandEscapes(%q) via YAML = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSafe_AppendReset(t *testing.T) {
	path := writeYAML(t, "my_key: \"CUSTOM\"\n")
	p := translations.NewYAMLProviderFromPath(path)

	// Translated value should have reset appended.
	got := translations.Safe(p, "my_key", "DEFAULT")
	want := "CUSTOM\x1b[0m"
	if got != want {
		t.Errorf("Safe translated = %q, want %q", got, want)
	}

	// Fallback (key absent) should NOT have reset appended.
	got = translations.Safe(p, "absent_key", "FALLBACK")
	if got != "FALLBACK" {
		t.Errorf("Safe fallback = %q, want %q", got, "FALLBACK")
	}
}

func TestSafe_NilProvider(t *testing.T) {
	got := translations.Safe(nil, "any_key", "FALLBACK")
	if got != "FALLBACK" {
		t.Errorf("Safe(nil) = %q, want %q", got, "FALLBACK")
	}
}

func TestNopProvider_ReturnsAll_Fallback(t *testing.T) {
	var p translations.NopProvider
	keys := []string{"a", "b", "c", translations.KeyCronTitle, translations.KeyHelpModalTitle}
	for _, k := range keys {
		if got := p.T(k, "FB"); got != "FB" {
			t.Errorf("NopProvider.T(%q) = %q, want %q", k, got, "FB")
		}
	}
}

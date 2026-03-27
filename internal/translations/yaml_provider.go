package translations

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLProvider loads translations from ~/.config/orcai/translations.yaml.
// The file is parsed as a flat map[string]string. Values may use the escape
// shorthand \e[, \033[, or \x1b[ which are expanded to raw ANSI bytes at
// load time.
type YAMLProvider struct {
	data map[string]string
}

// NewYAMLProvider loads ~/.config/orcai/translations.yaml and returns a
// Provider. If the file does not exist or cannot be parsed, it returns a
// NopProvider so callers always receive a valid non-nil Provider.
//
// The config directory is resolved as $HOME/.config to match the rest of
// the ORCAI tooling (themes, pipelines, etc.).
func NewYAMLProvider() Provider {
	home := os.Getenv("HOME")
	if home == "" {
		return NopProvider{}
	}
	path := filepath.Join(home, ".config", "orcai", "translations.yaml")
	return NewYAMLProviderFromPath(path)
}

// NewYAMLProviderFromPath loads a translations YAML file at an explicit path.
// It is provided for testing and for operators who want to load a custom file.
// The file must be a flat YAML map[string]string.
func NewYAMLProviderFromPath(path string) Provider {
	data, err := os.ReadFile(path)
	if err != nil {
		// Missing file is normal — return NopProvider silently.
		return NopProvider{}
	}

	var raw map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return NopProvider{}
	}

	expanded := make(map[string]string, len(raw))
	for k, v := range raw {
		expanded[k] = expandEscapes(v)
	}
	return &YAMLProvider{data: expanded}
}

// T returns the translation for key, or fallback if the key is absent.
func (y *YAMLProvider) T(key, fallback string) string {
	if v, ok := y.data[key]; ok {
		return v
	}
	return fallback
}

// expandEscapes converts escape shorthand notations to raw ANSI bytes.
func expandEscapes(s string) string {
	s = strings.ReplaceAll(s, `\e[`, "\x1b[")
	s = strings.ReplaceAll(s, `\033[`, "\x1b[")
	s = strings.ReplaceAll(s, `\x1b[`, "\x1b[")
	return s
}

package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/8op-org/gl1tch/internal/assets"
)

// LoadBundled reads all provider profiles from the embedded assets.ProviderFS.
// It returns the full set of bundled profiles (currently: claude, gemini,
// opencode, aider, goose, copilot).
func LoadBundled() ([]Profile, error) {
	entries, err := assets.ProviderFS.ReadDir("providers")
	if err != nil {
		return nil, fmt.Errorf("providers: read embedded dir: %w", err)
	}

	var profiles []Profile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := assets.ProviderFS.ReadFile("providers/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("providers: read embedded file %s: %w", e.Name(), err)
		}
		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("providers: parse embedded file %s: %w", e.Name(), err)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// LoadUser scans dir for *.yaml files and unmarshals each as a Profile.
// If dir does not exist, an empty slice and nil error are returned (same
// behaviour as plugin.LoadWrappers for missing user config directories).
func LoadUser(dir string) ([]Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("providers: read user dir %s: %w", dir, err)
	}

	var profiles []Profile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("providers: read user file %s: %w", path, err)
		}
		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("providers: parse user file %s: %w", path, err)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

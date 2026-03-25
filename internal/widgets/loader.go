package widgets

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Discover scans dir for subdirectories that contain a widget.yaml manifest
// and returns the parsed manifests. If dir does not exist, an empty slice and
// nil error are returned. A malformed manifest file causes an error to be
// returned immediately.
func Discover(dir string) ([]Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("widgets: read dir %s: %w", dir, err)
	}

	var manifests []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, e.Name(), ManifestFile)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Subdirectory has no widget.yaml — skip it.
				continue
			}
			return nil, fmt.Errorf("widgets: read manifest %s: %w", manifestPath, err)
		}
		var m Manifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("widgets: parse manifest %s: %w", manifestPath, err)
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

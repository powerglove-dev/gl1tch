package themes

import (
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/adam-stokes/orcai/internal/assets"
)

// LoadBundled reads all theme bundles embedded in assets.ThemeFS.
// The embedded FS contains themes at "themes/<name>/theme.yaml".
func LoadBundled() ([]Bundle, error) {
	return loadFromFS(assets.ThemeFS, "themes")
}

// LoadUser scans dir for subdirectory theme bundles (<dir>/<name>/theme.yaml).
// If dir does not exist, an empty slice is returned without error.
func LoadUser(dir string) ([]Bundle, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var bundles []Bundle
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		bundleDir := filepath.Join(dir, e.Name())
		yamlPath := filepath.Join(bundleDir, "theme.yaml")
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			// Skip directories that don't contain a theme.yaml.
			continue
		}
		var b Bundle
		if err := yaml.Unmarshal(data, &b); err != nil {
			return nil, err
		}
		// Populate HeaderBytes from .ans files relative to the bundle directory.
		b.HeaderBytes = make(map[string][][]byte)
		for key, paths := range b.Headers {
			var variants [][]byte
			for _, relPath := range paths {
				absPath := filepath.Join(bundleDir, relPath)
				if raw, err := os.ReadFile(absPath); err == nil {
					variants = append(variants, raw)
				}
			}
			if len(variants) > 0 {
				b.HeaderBytes[key] = variants
			}
		}
		bundles = append(bundles, b)
	}
	return bundles, nil
}

// loadFromFS is the common helper for reading themes out of an fs.FS rooted at
// root (e.g. "themes").  Each sub-directory is expected to contain theme.yaml.
func loadFromFS(fsys fs.FS, root string) ([]Bundle, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, err
	}
	var bundles []Bundle
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		bundleDir := root + "/" + e.Name()
		yamlPath := bundleDir + "/theme.yaml"
		data, err := fs.ReadFile(fsys, yamlPath)
		if err != nil {
			// Skip directories without a theme.yaml.
			continue
		}
		var b Bundle
		if err := yaml.Unmarshal(data, &b); err != nil {
			return nil, err
		}
		// Populate HeaderBytes from .ans files embedded in the FS.
		b.HeaderBytes = make(map[string][][]byte)
		for key, paths := range b.Headers {
			var variants [][]byte
			for _, relPath := range paths {
				ansPath := bundleDir + "/" + relPath
				if raw, err := fs.ReadFile(fsys, ansPath); err == nil {
					variants = append(variants, raw)
				}
			}
			if len(variants) > 0 {
				b.HeaderBytes[key] = variants
			}
		}
		bundles = append(bundles, b)
	}
	return bundles, nil
}

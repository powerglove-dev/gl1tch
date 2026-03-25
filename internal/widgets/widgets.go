// Package widgets implements discovery and launching of orcai widget binaries.
//
// Widgets are standalone binaries (any language) that orcai discovers from a
// directory of subdirectories, each containing a widget.yaml manifest. Once
// launched, a widget runs in its own tmux window and communicates with orcai
// via the Unix socket bus daemon (internal/busd).
package widgets

// ManifestFile is the filename that each widget subdirectory must contain.
const ManifestFile = "widget.yaml"

// Manifest describes a widget binary discovered from a widget.yaml file.
type Manifest struct {
	Name        string   `yaml:"name"`
	Binary      string   `yaml:"binary"`
	Description string   `yaml:"description"`
	Subscribe   []string `yaml:"subscribe"`
}

package pluginmanager

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFileName is the conventional name for a plugin manifest in a repo.
const ManifestFileName = "glitch-plugin.yaml"

// InstallMethod controls how the plugin binary is obtained.
type InstallMethod string

const (
	// InstallGo uses `go install <module>` to build and install the binary.
	InstallGo InstallMethod = "go"
	// InstallRelease downloads a pre-built binary from a GitHub release asset.
	InstallRelease InstallMethod = "release"
)

// PluginInstall describes how to obtain the plugin binary.
type PluginInstall struct {
	// Go is the Go module path for `go install`, e.g. "github.com/foo/bar/cmd/baz".
	// Used when Method is "go".
	Go string `yaml:"go"`
	// Release, if true, downloads from GitHub Releases. Infers asset name from
	// binary + GOOS + GOARCH. Used when Method is "release".
	Release bool `yaml:"release"`
}

// method returns the install method implied by the install block.
func (i PluginInstall) method() InstallMethod {
	if i.Release {
		return InstallRelease
	}
	return InstallGo
}

// PluginSignal declares a BUSD topic subscription and the named handler that
// gl1tch should invoke when the topic fires.
type PluginSignal struct {
	Topic   string `yaml:"topic"`
	Handler string `yaml:"handler"`
}

// PluginSidecar is an inline sidecar definition embedded in the plugin manifest.
// Fields map 1:1 to executor.SidecarSchema.
type PluginSidecar struct {
	Command      string         `yaml:"command"`
	Args         []string       `yaml:"args"`
	Description  string         `yaml:"description"`
	Category     string         `yaml:"category"`
	Kind         string         `yaml:"kind"` // "agent", "tool", or "daemon"
	// Daemon, when true, marks the plugin as a long-running background process.
	// gl1tch will start it automatically on session launch (after BUSD is ready)
	// and leave it running for the lifetime of the session.
	Daemon       bool           `yaml:"daemon,omitempty"`
	// Display describes the graphical requirements of a daemon plugin.
	// Valid values: "" or "headless" (always launched), "systray" (skipped on
	// headless hosts that have no windowing environment).
	Display      string         `yaml:"display,omitempty"`
	InputSchema  string         `yaml:"input_schema"`
	OutputSchema string         `yaml:"output_schema"`
	Signals      []PluginSignal `yaml:"signals,omitempty"`
}

// PluginManifest is the schema for a glitch-plugin.yaml file in a plugin repo.
type PluginManifest struct {
	// Name is the canonical plugin name (used as executor name and registry key).
	Name string `yaml:"name"`
	// Description is a short human-readable description.
	Description string `yaml:"description"`
	// Binary is the name of the installed binary on PATH. Defaults to Name.
	Binary string `yaml:"binary"`
	// Version is the pinned version / git ref. Optional; defaults to "latest".
	Version string `yaml:"version"`
	// Install describes how to obtain the binary.
	Install PluginInstall `yaml:"install"`
	// Sidecar is the inline executor configuration. If absent, minimal defaults
	// are derived from Name and Binary.
	Sidecar PluginSidecar `yaml:"sidecar"`

	// source is the resolved GitHub owner/repo, set by ParseRef.
	source string
}

// ParseManifest decodes a glitch-plugin.yaml from raw bytes.
func ParseManifest(data []byte) (*PluginManifest, error) {
	var m PluginManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("plugin manifest: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("plugin manifest: name is required")
	}
	if m.Binary == "" {
		m.Binary = m.Name
	}
	if m.Install.method() == InstallGo && m.Install.Go == "" {
		return nil, fmt.Errorf("plugin manifest %q: install.go is required when using go install", m.Name)
	}
	return &m, nil
}

// LoadManifestFromFile reads and parses a manifest from disk.
func LoadManifestFromFile(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	return ParseManifest(data)
}

// PluginRef is a parsed reference to a plugin, e.g. "owner/repo" or a full GitHub URL.
type PluginRef struct {
	Owner   string
	Repo    string
	Version string // empty means "latest" / default branch
}

// String returns the canonical "owner/repo[@version]" form.
func (r PluginRef) String() string {
	if r.Version != "" {
		return r.Owner + "/" + r.Repo + "@" + r.Version
	}
	return r.Owner + "/" + r.Repo
}

// ParseRef parses a plugin reference into owner/repo and optional version.
// Accepted forms:
//   - owner/repo
//   - owner/repo@v1.2.3
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo@v1.2.3  (non-standard but friendly)
func ParseRef(ref string) (PluginRef, error) {
	ref = strings.TrimSpace(ref)

	// Strip version suffix before URL parsing.
	version := ""
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		version = ref[idx+1:]
		ref = ref[:idx]
	}

	if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		u, err := url.Parse(ref)
		if err != nil {
			return PluginRef{}, fmt.Errorf("parse ref: invalid URL %q: %w", ref, err)
		}
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
		if len(parts) < 2 {
			return PluginRef{}, fmt.Errorf("parse ref: URL %q must have owner/repo path", ref)
		}
		return PluginRef{Owner: parts[0], Repo: parts[1], Version: version}, nil
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return PluginRef{}, fmt.Errorf("parse ref: %q must be owner/repo or a GitHub URL", ref)
	}
	return PluginRef{Owner: parts[0], Repo: parts[1], Version: version}, nil
}

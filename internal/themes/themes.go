// Package themes provides theme bundle loading and registry for orcai.
// Theme bundles contain color palettes, border styles, status bar configuration,
// and optional ANSI splash art.
//
// # Bus event
//
// When the active theme changes, callers should publish the following event to
// the bus:
//
//	&pb.Event{
//	    Topic:   TopicThemeChanged,
//	    Source:  "themes",
//	    Payload: []byte(newThemeName),
//	}
//
// Subscribers interested in theme changes should subscribe to [TopicThemeChanged].
package themes

// TopicThemeChanged is the bus topic published when the active theme changes.
// Payload is the new theme name as a plain UTF-8 string.
const TopicThemeChanged = "theme.changed"

// Bundle is a complete theme bundle loaded from a theme.yaml manifest.
type Bundle struct {
	Name        string    `yaml:"name"`
	DisplayName string    `yaml:"display_name"`
	Palette     Palette   `yaml:"palette"`
	Borders     Borders   `yaml:"borders"`
	StatusBar   StatusBar `yaml:"statusbar"`
	Splash      string    `yaml:"splash"` // relative path to .ans file within bundle
}

// Palette holds the seven canonical color slots for a theme.
type Palette struct {
	BG      string `yaml:"bg"`
	FG      string `yaml:"fg"`
	Accent  string `yaml:"accent"`
	Dim     string `yaml:"dim"`
	Border  string `yaml:"border"`
	Error   string `yaml:"error"`
	Success string `yaml:"success"`
}

// Borders configures the border drawing style.
type Borders struct {
	Style string `yaml:"style"` // "light", "heavy", or "ascii"
}

// StatusBar configures the status bar appearance.
// BG and FG are stored as-is from YAML (may reference palette keys like
// "palette.bg"); palette resolution is the caller's responsibility.
type StatusBar struct {
	Format string `yaml:"format"`
	BG     string `yaml:"bg"`
	FG     string `yaml:"fg"`
}

// ThemeChangedPayload is the structured payload for a theme.changed bus event.
// Callers may JSON-encode this struct into pb.Event.Payload, or use the theme
// name directly as plain bytes — both conventions are acceptable.
type ThemeChangedPayload struct {
	Name string `json:"name"`
}

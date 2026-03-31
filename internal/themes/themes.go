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

// Modal configures the modal overlay appearance.
// BG, Border, TitleBG, and TitleFG may contain palette references like "palette.accent";
// use [Bundle.ResolveRef] to expand them to hex color values before rendering.
type Modal struct {
	BG      string `yaml:"bg"`
	Border  string `yaml:"border"`
	TitleBG string `yaml:"title_bg"`
	TitleFG string `yaml:"title_fg"`
}

// PanelHeaderStyle configures the colors used when dynamically generating a
// panel header at the correct panel width. Both fields accept palette
// references (e.g. "palette.accent") or literal hex colors ("#rrggbb").
type PanelHeaderStyle struct {
	Accent string `yaml:"accent"` // border/bar color
	Text   string `yaml:"text"`   // title text color
	// GradientBorder overrides the bundle-level gradient for this specific panel.
	// When empty, falls back to Bundle.GradientBorder.
	GradientBorder []string `yaml:"gradient_border"`
}

// HeaderStyle configures the dynamic panel header generator.
// TopChar/BotChar/BorderChar default to "▄"/"▀"/"█" when empty.
// Panels maps each panel key to its color pair.
type HeaderStyle struct {
	TopChar    string                      `yaml:"top_char"`
	BotChar    string                      `yaml:"bot_char"`
	BorderChar string                      `yaml:"border_char"`
	Panels     map[string]PanelHeaderStyle `yaml:"panels"`
}

// Bundle is a complete theme bundle loaded from a theme.yaml manifest.
type Bundle struct {
	Name        string      `yaml:"name"`
	DisplayName string      `yaml:"display_name"`
	Mode        string      `yaml:"mode"` // "dark" or "light"
	Palette     Palette     `yaml:"palette"`
	Borders     Borders     `yaml:"borders"`
	StatusBar   StatusBar   `yaml:"statusbar"`
	Splash      string      `yaml:"splash"` // relative path to .ans file within bundle
	Modal       Modal       `yaml:"modal"`

	// HeaderStyle drives dynamic header generation at the exact panel width.
	// Used by DynamicHeader() when no fixed-width .ans sprite fits the panel.
	HeaderStyle HeaderStyle `yaml:"header_style"`

	// GradientBorder holds 0–4 hex color stops for gradient panel borders.
	// When empty, solid accent color is used. When 1 stop, solid that color.
	GradientBorder []string `yaml:"gradient_border"`

	// Headers maps panel key → ordered list of relative .ans paths, widest first.
	// SpriteLines selects the first entry whose visible width fits the panel.
	// Optional: omit to rely entirely on HeaderStyle dynamic generation.
	Headers     map[string][]string `yaml:"headers"`

	// HeaderBytes is populated by the loader after YAML unmarshal — not from YAML.
	// Keys match Headers; each inner slice mirrors the order of Headers[key].
	HeaderBytes map[string][][]byte `yaml:"-"`

	// HeaderPattern names a pattern from styles.Patterns for decorative header rows.
	// Empty string uses the default ▄/▀ block character rendering.
	HeaderPattern string `yaml:"header_pattern"`

	// Strings holds optional per-theme UI string overrides. These sit between
	// the user's translations.yaml and the built-in defaults in the provider
	// chain, letting theme authors ship copy that matches their aesthetic.
	// Keys match the constants in the translations package.
	Strings map[string]string `yaml:"strings"`
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
// BG and FG may contain palette references like "palette.bg"; use
// [Bundle.ResolveRef] to expand them to hex color values before rendering.
type StatusBar struct {
	Format string `yaml:"format"`
	BG     string `yaml:"bg"`
	FG     string `yaml:"fg"`
}

// ResolveRef expands a palette reference string to its hex color value.
// If val starts with "palette.", the remainder is matched case-insensitively
// against the palette field names (bg, fg, accent, dim, border, error, success).
// If val does not match a reference or is not found, it is returned unchanged.
func (b *Bundle) ResolveRef(val string) string {
	const prefix = "palette."
	if len(val) <= len(prefix) || val[:len(prefix)] != prefix {
		return val
	}
	key := val[len(prefix):]
	switch key {
	case "bg":
		return b.Palette.BG
	case "fg":
		return b.Palette.FG
	case "accent":
		return b.Palette.Accent
	case "dim":
		return b.Palette.Dim
	case "border":
		return b.Palette.Border
	case "error":
		return b.Palette.Error
	case "success":
		return b.Palette.Success
	}
	return val
}

// ThemeChangedPayload is the structured payload for a theme.changed bus event.
// Encode this struct as JSON into pb.Event.Payload when publishing.
type ThemeChangedPayload struct {
	Name string `json:"name"`
}

// Package translations provides the UI text and personality customization layer
// for GL1TCH. Users can override any labeled string — panel titles, modal
// messages, onboarding scripts, help text — via ~/.config/glitch/translations.yaml.
// Values may contain raw ANSI escape sequences or the shorthand notations
// \e[, \033[, and \x1b[.
package translations

// Provider is implemented by any translations source.
type Provider interface {
	// T returns the translation for key, or fallback if key is not configured.
	T(key, fallback string) string
}

package translations

// MapProvider is a Provider backed by a plain map[string]string.
// Used for theme-bundled strings and the built-in default voice layer.
// ANSI escape shorthands (\e[, \033[, \x1b[) are expanded at construction time.
type MapProvider struct {
	data map[string]string
}

// NewMapProvider returns a MapProvider for the given map. A nil or empty map
// produces a valid provider that returns fallback for every key.
// expandEscapes is applied to every value at construction time.
func NewMapProvider(m map[string]string) Provider {
	if len(m) == 0 {
		return NopProvider{}
	}
	expanded := make(map[string]string, len(m))
	for k, v := range m {
		expanded[k] = expandEscapes(v)
	}
	return MapProvider{data: expanded}
}

// T returns the translation for key, or fallback if the key is absent.
func (p MapProvider) T(key, fallback string) string {
	if v, ok := p.data[key]; ok {
		return v
	}
	return fallback
}

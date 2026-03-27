package translations

// NopProvider is a no-op implementation of Provider.
// It always returns the fallback value unchanged. Used when no translations
// file is present or when translations are not configured.
type NopProvider struct{}

// T always returns fallback.
func (NopProvider) T(_, fallback string) string { return fallback }

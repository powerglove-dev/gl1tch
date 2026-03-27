package translations

// Safe looks up key via p and appends an ANSI reset sequence (\x1b[0m) when
// the returned value differs from fallback (i.e. a custom translation was
// found). Fallback strings are returned as-is so callers that never load a
// translations file see no change in behavior.
//
// If p is nil, fallback is returned unchanged.
func Safe(p Provider, key, fallback string) string {
	if p == nil {
		return fallback
	}
	v := p.T(key, fallback)
	if v == fallback {
		return v // unchanged — don't append reset
	}
	return v + "\x1b[0m"
}

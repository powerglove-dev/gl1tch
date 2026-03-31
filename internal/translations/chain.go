package translations

// ChainProvider tries each provider in order and returns the first value that
// differs from the fallback. If no provider has the key, fallback is returned.
// This enables a priority stack: user overrides → theme strings → defaults.
type ChainProvider struct {
	providers []Provider
}

// NewChain returns a ChainProvider that consults each provider in order.
// Providers listed first have higher priority.
func NewChain(providers ...Provider) Provider {
	return ChainProvider{providers: providers}
}

// T walks the provider chain and returns the first translation that is not
// equal to fallback, or fallback itself if no provider has the key.
func (c ChainProvider) T(key, fallback string) string {
	for _, p := range c.providers {
		if p == nil {
			continue
		}
		if v := p.T(key, fallback); v != fallback {
			return v
		}
	}
	return fallback
}

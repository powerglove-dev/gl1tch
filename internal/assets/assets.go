// Package assets provides embedded provider profiles and theme bundles for the
// orcai plugin system. Files are embedded at compile time using //go:embed so
// no filesystem reads are required at runtime.
package assets

import "embed"

// ProviderFS contains all bundled provider YAML profiles.
//
//go:embed providers/*.yaml
var ProviderFS embed.FS

// ThemeFS contains all bundled theme bundles, including manifests and assets.
//
//go:embed themes
var ThemeFS embed.FS

// ExamplesFS contains example configuration files for end-user reference.
//
//go:embed examples
var ExamplesFS embed.FS

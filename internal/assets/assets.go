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

// FontFS contains embedded TDF font files for ANSI block-letter header rendering.
//
//go:embed fonts/tdf/*.tdf
var FontFS embed.FS

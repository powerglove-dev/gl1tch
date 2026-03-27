package tdf

import (
	"io/fs"
	"strings"
	"sync"

	"github.com/adam-stokes/orcai/internal/assets"
)

// FontRegistry maps font names to parsed Font instances.
type FontRegistry struct {
	mu    sync.RWMutex
	fonts map[string]*Font
}

// Global is the process-level font registry, populated once at startup from
// the embedded TDF font files.
var Global = &FontRegistry{fonts: make(map[string]*Font)}

func init() {
	// Best-effort: ignore errors so a missing embed doesn't crash startup.
	_ = Global.LoadFS(assets.FontFS, "fonts/tdf")
}

// LoadFS loads all .tdf files from fsys rooted at dir into the registry.
// Files that fail to parse are silently skipped.
func (r *FontRegistry) LoadFS(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tdf") {
			continue
		}
		data, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			continue
		}
		f, err := Parse(data)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".tdf")
		r.mu.Lock()
		r.fonts[name] = f
		r.mu.Unlock()
	}
	return nil
}

// Get returns the named font, or nil if not found.
func (r *FontRegistry) Get(name string) *Font {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fonts[name]
}

// Render renders text with the named font at maxWidth.
// Falls back to plain text if the font is not found or rendering fails.
func (r *FontRegistry) Render(name, text string, maxWidth int) string {
	f := r.Get(name)
	if f == nil {
		return text
	}
	result, err := f.Render(text, maxWidth)
	if err != nil {
		return text
	}
	return result
}

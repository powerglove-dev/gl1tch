package search

import (
	"strings"

	"github.com/8op-org/gl1tch/internal/chatui"
	"github.com/blevesearch/bleve/v2"
)

// doc is the bleve-indexed representation of an IndexEntry.
type doc struct {
	Name        string
	Kind        string
	Source      string
	Description string
	Inject      string
}

// Query searches entries using BM25 full-text matching against name, kind, source,
// description, and inject fields. Returns all entries unchanged if query is empty.
// Uses an in-memory bleve index rebuilt per call (corpus is small, ~50-200 items).
//
// TODO(Task 5): wire Query as a SearchIndex bound method on the Wails App struct in app.go.
func Query(entries []chatui.IndexEntry, query string) []chatui.IndexEntry {
	if query == "" {
		return entries
	}

	mapping := bleve.NewIndexMapping()
	idx, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return substringFilter(entries, query)
	}
	defer idx.Close()

	for _, e := range entries {
		_ = idx.Index(e.Name, doc{
			Name:        e.Name,
			Kind:        e.Kind,
			Source:      e.Source,
			Description: e.Description,
			Inject:      e.Inject,
		})
	}

	// Use MatchQuery (not QueryStringQuery) so raw user input with special
	// characters like "-", ":", "?" doesn't get parsed as bleve operators.
	q := bleve.NewMatchQuery(query)
	req := bleve.NewSearchRequest(q)
	req.Size = len(entries)
	res, err := idx.Search(req)
	if err != nil || res.Total == 0 {
		return substringFilter(entries, query)
	}

	nameMap := make(map[string]chatui.IndexEntry, len(entries))
	for _, e := range entries {
		nameMap[e.Name] = e
	}
	out := make([]chatui.IndexEntry, 0, len(res.Hits))
	for _, hit := range res.Hits {
		if e, ok := nameMap[hit.ID]; ok {
			out = append(out, e)
		}
	}
	return out
}

// substringFilter is the fallback for empty results or bleve errors.
func substringFilter(entries []chatui.IndexEntry, query string) []chatui.IndexEntry {
	q := strings.ToLower(query)
	var out []chatui.IndexEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), q) ||
			strings.Contains(strings.ToLower(e.Description), q) {
			out = append(out, e)
		}
	}
	return out
}

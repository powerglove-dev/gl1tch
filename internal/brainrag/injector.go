package brainrag

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/8op-org/gl1tch/internal/braincontext"
	"github.com/8op-org/gl1tch/internal/store"
)

// BrainInjector queries the RAG store and injects relevant brain notes into prompts.
type BrainInjector struct {
	RAG          *RAGStore
	Store        *store.Store
	WorkspaceCtx braincontext.WorkspaceContext
	TopK         int    // default 5
	BaseURL      string // ollama base URL
	Model        string // embedding model
}

// InjectInto queries the RAG store using prompt as the query text, fetches the
// full note bodies for the top-K results, and prepends them to the prompt as:
//
//	## Relevant Brain Notes
//	[note-id] body...
//	---
//
// Returns the original prompt unmodified if RAG/store is unavailable.
func (b *BrainInjector) InjectInto(ctx context.Context, prompt string) (string, error) {
	if b.RAG == nil || b.Store == nil {
		return prompt, nil
	}

	topK := b.TopK
	if topK <= 0 {
		topK = 5
	}
	baseURL := b.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	model := b.Model
	if model == "" {
		model = DefaultEmbedModel
	}

	// Use linked note IDs as filter when workspace context is set.
	var filter []string
	if len(b.WorkspaceCtx.LinkedNoteIDs) > 0 {
		filter = b.WorkspaceCtx.LinkedNoteIDs
	}

	noteIDs, err := b.RAG.Query(ctx, baseURL, model, prompt, topK, filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[brainrag] warn: RAG query failed: %v\n", err)
		return prompt, nil
	}
	if len(noteIDs) == 0 {
		return prompt, nil
	}

	// Fetch note bodies from the store.
	// We fetch all recent notes and index by ID string.
	allNotes, err := b.Store.AllBrainNotes(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[brainrag] warn: fetch brain notes failed: %v\n", err)
		return prompt, nil
	}

	noteMap := make(map[string]store.BrainNote, len(allNotes))
	for _, n := range allNotes {
		noteMap[fmt.Sprintf("%d", n.ID)] = n
	}

	var sb strings.Builder
	found := 0
	hasCodeChunks := false
	for _, id := range noteIDs {
		if strings.HasPrefix(id, "file:") {
			hasCodeChunks = true
			break
		}
	}

	if hasCodeChunks {
		sb.WriteString("## Relevant Source Code\n\n")
		sb.WriteString("The following code chunks were retrieved by semantic search over the indexed codebase.\n")
		sb.WriteString("These are the most relevant matches for this task — not an exhaustive listing.\n")
		sb.WriteString("Focus only on source files shown. Ignore generated files, vendor directories,\n")
		sb.WriteString("hidden files/directories, build output, and test fixtures — those are excluded from the index.\n")
		sb.WriteString("If the relevant code is not shown here, it may not have been indexed yet (run builtin.index_code to refresh).\n\n")
	} else {
		sb.WriteString("## Relevant Brain Notes\n\n")
	}

	for _, id := range noteIDs {
		n, ok := noteMap[id]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n\n", id, n.Body))
		found++
	}
	if found == 0 {
		return prompt, nil
	}
	sb.WriteString("---\n")
	return sb.String() + prompt, nil
}

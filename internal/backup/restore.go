package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/8op-org/gl1tch/internal/store"
)

// RestoreOptions configures a restore operation.
type RestoreOptions struct {
	Overwrite bool
	DryRun    bool
	ConfigDir string // override ~/.config/glitch
}

// RestoreSummary describes what happened during restore.
type RestoreSummary struct {
	FilesWritten     int
	FilesSkipped     int
	FilesOverwritten int
	NotesImported    int
	NotesSkipped     int
	PromptsImported  int
	PromptsSkipped   int
}

// Restore unpacks a backup archive into the user's config and database.
func Restore(ctx context.Context, s *store.Store, archivePath string, opts RestoreOptions) (*RestoreSummary, error) {
	configDir, err := resolveConfigDir(opts.ConfigDir)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("restore: open archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("restore: not a valid gzip archive: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	summary := &RestoreSummary{}

	var brainNotes []store.BrainNote
	var prompts []store.Prompt

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("restore: read archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}

		switch {
		case strings.HasPrefix(hdr.Name, "config/"):
			rel := strings.TrimPrefix(hdr.Name, "config/")
			destPath := filepath.Join(configDir, filepath.FromSlash(rel))
			if err := restoreFile(tr, destPath, opts, summary); err != nil {
				return nil, err
			}
		case hdr.Name == "db/brain_notes.jsonl":
			notes, err := decodeJSONL[store.BrainNote](tr)
			if err != nil {
				return nil, fmt.Errorf("restore: decode brain_notes.jsonl: %w", err)
			}
			brainNotes = notes
		case hdr.Name == "db/saved_prompts.jsonl":
			ps, err := decodeJSONL[store.Prompt](tr)
			if err != nil {
				return nil, fmt.Errorf("restore: decode saved_prompts.jsonl: %w", err)
			}
			prompts = ps
		}
	}

	for _, note := range brainNotes {
		exists, err := rowExists(ctx, s, `SELECT COUNT(*) FROM brain_notes WHERE id = ?`, note.ID)
		if err != nil {
			return nil, err
		}
		if exists {
			summary.NotesSkipped++
			continue
		}
		if !opts.DryRun {
			if err := s.UpsertBrainNote(ctx, note); err != nil {
				return nil, err
			}
		}
		summary.NotesImported++
	}

	for _, p := range prompts {
		exists, err := rowExists(ctx, s, `SELECT COUNT(*) FROM prompts WHERE id = ?`, p.ID)
		if err != nil {
			return nil, err
		}
		if exists {
			summary.PromptsSkipped++
			continue
		}
		if !opts.DryRun {
			if err := s.UpsertPrompt(ctx, p); err != nil {
				return nil, err
			}
		}
		summary.PromptsImported++
	}

	return summary, nil
}

func restoreFile(r io.Reader, destPath string, opts RestoreOptions, summary *RestoreSummary) error {
	_, statErr := os.Stat(destPath)
	exists := statErr == nil

	if exists && !opts.Overwrite {
		summary.FilesSkipped++
		return nil
	}

	if opts.DryRun {
		if exists {
			summary.FilesOverwritten++
		} else {
			summary.FilesWritten++
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("restore: mkdir %s: %w", filepath.Dir(destPath), err)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("restore: create %s: %w", destPath, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("restore: write %s: %w", destPath, err)
	}
	if exists {
		summary.FilesOverwritten++
	} else {
		summary.FilesWritten++
	}
	return nil
}

func decodeJSONL[T any](r io.Reader) ([]T, error) {
	var items []T
	dec := json.NewDecoder(r)
	for dec.More() {
		var item T
		if err := dec.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func rowExists(ctx context.Context, s *store.Store, query string, args ...any) (bool, error) {
	var count int
	if err := s.DB().QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("restore: existence check: %w", err)
	}
	return count > 0, nil
}

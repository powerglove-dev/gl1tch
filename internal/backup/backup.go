// Package backup implements glitch backup and restore commands.
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/8op-org/gl1tch/internal/store"
)

// BackupOptions configures a backup run.
type BackupOptions struct {
	Output        string // output path; empty = auto-generated in cwd
	ConfigDir     string // override ~/.config/glitch
	DBPath        string // override ~/.local/share/glitch/glitch.db
	ExcludeAgents bool   // historical: previously skipped pipelines/.agents/ (no-op now)
}

// Manifest describes what was included in the backup.
type Manifest struct {
	Path        string
	FileCount   int
	NoteCount   int
	PromptCount int
}

// configTargets lists the paths within the config dir to include in the archive.
// Workflows are no longer stored under the global config dir — they live in
// each workspace under .glitch/workflows/ and are tracked by the workspace's
// own VCS, so they're not part of the gl1tch backup.
var configTargets = []string{
	"prompts",
	"wrappers",
	"themes",
	"cron.yaml",
	"layout.yaml",
	"config.yaml",
	"translations.yaml",
}

// Run creates a backup archive at opts.Output (or an auto-generated path).
func Run(ctx context.Context, s *store.Store, opts BackupOptions) (*Manifest, error) {
	configDir, err := resolveConfigDir(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	dbPath, err := resolveDBPath(opts.DBPath)
	if err != nil {
		return nil, err
	}
	outPath, err := resolveOutputPath(opts.Output)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(outPath); err == nil {
		return nil, fmt.Errorf("backup: output file already exists: %s", outPath)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("backup: create archive: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	var fileCount int

	var excludeDirs []string
	_ = opts.ExcludeAgents // historical option; the .agents dir no longer exists.

	for _, target := range configTargets {
		targetPath := filepath.Join(configDir, target)
		info, statErr := os.Stat(targetPath)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return nil, fmt.Errorf("backup: stat %s: %w", target, statErr)
		}
		if info.IsDir() {
			n, err := addDir(tw, targetPath, "config/"+target, excludeDirs)
			if err != nil {
				return nil, err
			}
			fileCount += n
		} else {
			if err := addFile(tw, targetPath, "config/"+target); err != nil {
				return nil, err
			}
			fileCount++
		}
	}

	notes, err := s.AllBrainNotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("backup: load brain notes: %w", err)
	}
	if err := addJSONL(tw, "db/brain_notes.jsonl", toAnySlice(notes)); err != nil {
		return nil, err
	}

	prompts, err := s.ListPrompts(ctx)
	if err != nil {
		return nil, fmt.Errorf("backup: load prompts: %w", err)
	}
	if err := addJSONL(tw, "db/saved_prompts.jsonl", toAnySlice(prompts)); err != nil {
		return nil, err
	}

	// Raw DB copy — non-fatal if locked or missing; JSONL is the primary export.
	_ = addFile(tw, dbPath, "db/glitch.db")

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("backup: close tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("backup: close gzip: %w", err)
	}

	return &Manifest{
		Path:        outPath,
		FileCount:   fileCount,
		NoteCount:   len(notes),
		PromptCount: len(prompts),
	}, nil
}

func resolveConfigDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("backup: resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "glitch"), nil
}

func resolveDBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("backup: resolve home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "glitch", "glitch.db"), nil
}

func resolveOutputPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	date := time.Now().Format("2006-01-02")
	return fmt.Sprintf("glitch-backup-%s.tar.gz", date), nil
}

func addDir(tw *tar.Writer, srcDir, archivePrefix string, excludeDirs []string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			for _, ex := range excludeDirs {
				if path == ex {
					return fs.SkipDir
				}
			}
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		archivePath := archivePrefix + "/" + filepath.ToSlash(rel)
		if err := addFile(tw, path, archivePath); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func addFile(tw *tar.Writer, srcPath, archivePath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("backup: open %s: %w", srcPath, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("backup: stat %s: %w", srcPath, err)
	}
	hdr := &tar.Header{
		Name:    archivePath,
		Mode:    int64(info.Mode()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("backup: write header %s: %w", archivePath, err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("backup: copy %s: %w", archivePath, err)
	}
	return nil
}

func addJSONL(tw *tar.Writer, archivePath string, items []any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return fmt.Errorf("backup: encode jsonl %s: %w", archivePath, err)
		}
	}
	hdr := &tar.Header{
		Name:    archivePath,
		Mode:    0o644,
		Size:    int64(buf.Len()),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("backup: write header %s: %w", archivePath, err)
	}
	if _, err := io.Copy(tw, &buf); err != nil {
		return fmt.Errorf("backup: write jsonl %s: %w", archivePath, err)
	}
	return nil
}

func toAnySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

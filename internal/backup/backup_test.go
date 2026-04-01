package backup_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/backup"
	"github.com/8op-org/gl1tch/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makeConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pipelines := filepath.Join(dir, "pipelines")
	if err := os.MkdirAll(pipelines, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelines, "test.pipeline.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("store:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func archiveEntries(t *testing.T, archivePath string) map[string]string {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	entries := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read entry %s: %v", hdr.Name, err)
		}
		entries[hdr.Name] = string(b)
	}
	return entries
}

// Task 5.1: Run() produces archive with expected entries.
func TestBackupRun_ArchiveContents(t *testing.T) {
	s := openTestStore(t)
	configDir := makeConfigDir(t)
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	ctx := context.Background()

	// Insert a brain note so it appears in the archive.
	_, err := s.InsertBrainNote(ctx, store.BrainNote{
		RunID: 1, StepID: "s1", CreatedAt: 1000, Tags: "test", Body: "hello brain",
	})
	if err != nil {
		t.Fatalf("insert brain note: %v", err)
	}

	m, err := backup.Run(ctx, s, backup.BackupOptions{
		Output:    outPath,
		ConfigDir: configDir,
	})
	if err != nil {
		t.Fatalf("backup.Run: %v", err)
	}

	if m.Path != outPath {
		t.Errorf("manifest path = %q, want %q", m.Path, outPath)
	}
	if m.NoteCount != 1 {
		t.Errorf("NoteCount = %d, want 1", m.NoteCount)
	}

	entries := archiveEntries(t, outPath)

	if _, ok := entries["config/pipelines/test.pipeline.yaml"]; !ok {
		t.Error("archive missing config/pipelines/test.pipeline.yaml")
	}
	if _, ok := entries["config/config.yaml"]; !ok {
		t.Error("archive missing config/config.yaml")
	}
	if _, ok := entries["db/brain_notes.jsonl"]; !ok {
		t.Error("archive missing db/brain_notes.jsonl")
	}
	if _, ok := entries["db/saved_prompts.jsonl"]; !ok {
		t.Error("archive missing db/saved_prompts.jsonl")
	}
}

// Task 5.1 (continued): Run() returns error if output already exists.
func TestBackupRun_OutputExists(t *testing.T) {
	s := openTestStore(t)
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := os.WriteFile(outPath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := backup.Run(context.Background(), s, backup.BackupOptions{
		Output:    outPath,
		ConfigDir: t.TempDir(),
	})
	if err == nil {
		t.Error("expected error when output file already exists")
	}
}

// Task 5.2: Restore writes new files and skips existing ones (default).
func TestRestore_SkipExisting(t *testing.T) {
	s := openTestStore(t)
	configDir := makeConfigDir(t)
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	ctx := context.Background()
	if _, err := backup.Run(ctx, s, backup.BackupOptions{
		Output:    outPath,
		ConfigDir: configDir,
	}); err != nil {
		t.Fatalf("backup.Run: %v", err)
	}

	restoreDir := t.TempDir()

	// First restore — all files are new.
	sum, err := backup.Restore(ctx, s, outPath, backup.RestoreOptions{ConfigDir: restoreDir})
	if err != nil {
		t.Fatalf("first restore: %v", err)
	}
	if sum.FilesWritten == 0 {
		t.Error("expected at least one file written on first restore")
	}
	if sum.FilesSkipped != 0 {
		t.Errorf("expected 0 skipped on first restore, got %d", sum.FilesSkipped)
	}

	// Second restore — all files already exist; expect all skipped.
	sum2, err := backup.Restore(ctx, s, outPath, backup.RestoreOptions{ConfigDir: restoreDir})
	if err != nil {
		t.Fatalf("second restore: %v", err)
	}
	if sum2.FilesSkipped == 0 {
		t.Error("expected files to be skipped on second restore")
	}
	if sum2.FilesWritten != 0 {
		t.Errorf("expected 0 written on second restore, got %d", sum2.FilesWritten)
	}
}

// Task 5.2 (continued): Restore overwrites existing files when --overwrite is set.
func TestRestore_Overwrite(t *testing.T) {
	s := openTestStore(t)
	configDir := makeConfigDir(t)
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	ctx := context.Background()
	if _, err := backup.Run(ctx, s, backup.BackupOptions{
		Output:    outPath,
		ConfigDir: configDir,
	}); err != nil {
		t.Fatalf("backup.Run: %v", err)
	}

	restoreDir := t.TempDir()
	// First restore to populate.
	if _, err := backup.Restore(ctx, s, outPath, backup.RestoreOptions{ConfigDir: restoreDir}); err != nil {
		t.Fatalf("first restore: %v", err)
	}

	// Second restore with --overwrite.
	sum, err := backup.Restore(ctx, s, outPath, backup.RestoreOptions{ConfigDir: restoreDir, Overwrite: true})
	if err != nil {
		t.Fatalf("overwrite restore: %v", err)
	}
	if sum.FilesOverwritten == 0 {
		t.Error("expected files to be overwritten")
	}
	if sum.FilesSkipped != 0 {
		t.Errorf("expected 0 skipped with --overwrite, got %d", sum.FilesSkipped)
	}
}

// Task 5.3: Dry-run reports changes without writing.
func TestRestore_DryRun(t *testing.T) {
	s := openTestStore(t)
	configDir := makeConfigDir(t)
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	ctx := context.Background()
	if _, err := backup.Run(ctx, s, backup.BackupOptions{
		Output:    outPath,
		ConfigDir: configDir,
	}); err != nil {
		t.Fatalf("backup.Run: %v", err)
	}

	restoreDir := t.TempDir()
	sum, err := backup.Restore(ctx, s, outPath, backup.RestoreOptions{ConfigDir: restoreDir, DryRun: true})
	if err != nil {
		t.Fatalf("dry-run restore: %v", err)
	}
	if sum.FilesWritten == 0 {
		t.Error("dry-run should report files that would be written")
	}

	// Confirm nothing was actually written.
	entries, err := os.ReadDir(restoreDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("dry-run wrote %d entries to restoreDir, want 0", len(entries))
	}
}

// Task 5.4: DB records with duplicate IDs are skipped on restore.
func TestRestore_DBDedup(t *testing.T) {
	s := openTestStore(t)
	configDir := makeConfigDir(t)
	outPath := filepath.Join(t.TempDir(), "backup.tar.gz")

	ctx := context.Background()

	id, err := s.InsertBrainNote(ctx, store.BrainNote{
		RunID: 1, StepID: "s1", CreatedAt: 1000, Tags: "", Body: "note body",
	})
	if err != nil {
		t.Fatalf("insert brain note: %v", err)
	}
	_ = id

	if _, err := backup.Run(ctx, s, backup.BackupOptions{
		Output:    outPath,
		ConfigDir: configDir,
	}); err != nil {
		t.Fatalf("backup.Run: %v", err)
	}

	// Restore into the same store — note already exists, should be skipped.
	sum, err := backup.Restore(ctx, s, outPath, backup.RestoreOptions{ConfigDir: t.TempDir()})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if sum.NotesSkipped != 1 {
		t.Errorf("NotesSkipped = %d, want 1", sum.NotesSkipped)
	}
	if sum.NotesImported != 0 {
		t.Errorf("NotesImported = %d, want 0", sum.NotesImported)
	}
}

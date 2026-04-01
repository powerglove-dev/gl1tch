package console

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadSet_MissingFile(t *testing.T) {
	set := LoadReadSet("/tmp/glitch-nonexistent-readstate-test.json")
	if len(set) != 0 {
		t.Errorf("expected empty set for missing file, got %v", set)
	}
}

func TestLoadReadSet_CorruptFile(t *testing.T) {
	f, err := os.CreateTemp("", "orcai-readstate-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("not valid json{{{")
	f.Close()

	set := LoadReadSet(f.Name())
	if len(set) != 0 {
		t.Errorf("expected empty set for corrupt file, got %v", set)
	}
}

func TestSaveAndLoadReadSet_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox-read.json")

	ids := map[int64]bool{1: true, 42: true, 999: true}
	if err := SaveReadSet(path, ids); err != nil {
		t.Fatalf("SaveReadSet: %v", err)
	}
	loaded := LoadReadSet(path)
	for id := range ids {
		if !loaded[id] {
			t.Errorf("expected ID %d in loaded set", id)
		}
	}
	if len(loaded) != len(ids) {
		t.Errorf("loaded set size %d != expected %d", len(loaded), len(ids))
	}
}

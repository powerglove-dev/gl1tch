package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/discovery"
)

func TestScanNative_Empty(t *testing.T) {
	dir := t.TempDir()
	executors, err := discovery.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, e := range executors {
		if e.Type == discovery.TypeNative {
			t.Errorf("expected no native executors in empty dir, got %+v", e)
		}
	}
}

func TestScanNative_FindsExecutable(t *testing.T) {
	dir := t.TempDir()
	executorsDir := filepath.Join(dir, "executors")
	if err := os.MkdirAll(executorsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(executorsDir, "glitch-test-executor")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hi"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	executors, err := discovery.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	found := false
	for _, e := range executors {
		if e.Name == "glitch-test-executor" && e.Type == discovery.TypeNative {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find glitch-test-executor, got %+v", executors)
	}
}

func TestScanNative_SkipsNonExecutable(t *testing.T) {
	dir := t.TempDir()
	executorsDir := filepath.Join(dir, "executors")
	if err := os.MkdirAll(executorsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(executorsDir, "not-executable")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	executors, err := discovery.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, e := range executors {
		if e.Name == "not-executable" {
			t.Errorf("should not have loaded non-executable file")
		}
	}
}

func TestNativePriorityOverCLI(t *testing.T) {
	dir := t.TempDir()
	executorsDir := filepath.Join(dir, "executors")
	if err := os.MkdirAll(executorsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Create a native executor named "claude" — it should shadow the CLI wrapper
	path := filepath.Join(executorsDir, "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	executors, err := discovery.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	count := 0
	for _, e := range executors {
		if e.Name == "claude" {
			count++
			if e.Type != discovery.TypeNative {
				t.Errorf("expected claude to be TypeNative, got %v", e.Type)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 claude executor, got %d", count)
	}
}


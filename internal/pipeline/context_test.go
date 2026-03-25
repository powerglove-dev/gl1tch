package pipeline_test

import (
	"testing"

	"github.com/adam-stokes/orcai/internal/pipeline"
)

func TestExecutionContext_SetGet(t *testing.T) {
	ec := pipeline.NewExecutionContext()
	ec.Set("param.env", "staging")
	v, ok := ec.Get("param.env")
	if !ok {
		t.Fatal("expected value at param.env")
	}
	if v != "staging" {
		t.Errorf("got %v, want staging", v)
	}
}

func TestExecutionContext_DotPath(t *testing.T) {
	ec := pipeline.NewExecutionContext()
	ec.Set("step.fetch.data.url", "https://example.com")
	v, ok := ec.Get("step.fetch.data.url")
	if !ok {
		t.Fatal("expected value at step.fetch.data.url")
	}
	if v != "https://example.com" {
		t.Errorf("got %v, want https://example.com", v)
	}
}

func TestExecutionContext_MissingPath(t *testing.T) {
	ec := pipeline.NewExecutionContext()
	_, ok := ec.Get("step.missing.data.key")
	if ok {
		t.Error("expected false for missing path")
	}
}

func TestExecutionContext_Snapshot(t *testing.T) {
	ec := pipeline.NewExecutionContext()
	ec.Set("key", "original")
	snap := ec.Snapshot()
	// Modify original after snapshot.
	ec.Set("key", "modified")
	// Snapshot top-level key should be unchanged.
	if snap["key"] != "original" {
		t.Errorf("snapshot should be independent; got %v", snap["key"])
	}
}

func TestExecutionContext_SetCreatesIntermediateMaps(t *testing.T) {
	ec := pipeline.NewExecutionContext()
	ec.Set("a.b.c", 42)
	v, ok := ec.Get("a.b.c")
	if !ok {
		t.Fatal("expected value at a.b.c")
	}
	if v != 42 {
		t.Errorf("got %v, want 42", v)
	}
}

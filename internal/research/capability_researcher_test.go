package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/8op-org/gl1tch/internal/capability"
)

// fakeCap is a controllable capability used by the adapter tests. It emits
// a fixed sequence of events on Invoke and optionally returns an error from
// Invoke itself.
type fakeCap struct {
	manifest  capability.Manifest
	events    []capability.Event
	invokeErr error
}

func (f *fakeCap) Manifest() capability.Manifest { return f.manifest }

func (f *fakeCap) Invoke(_ context.Context, _ capability.Input) (<-chan capability.Event, error) {
	if f.invokeErr != nil {
		return nil, f.invokeErr
	}
	ch := make(chan capability.Event, len(f.events))
	for _, ev := range f.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func TestCapabilityResearcherStreamEventsBecomeBody(t *testing.T) {
	cap := &fakeCap{
		manifest: capability.Manifest{
			Name:        "git",
			Description: "wraps the git capability",
		},
		events: []capability.Event{
			{Kind: capability.EventStream, Text: "commit a\n"},
			{Kind: capability.EventStream, Text: "commit b\n"},
		},
	}
	r := NewCapabilityResearcher(cap)
	if r.Name() != "git" {
		t.Errorf("Name = %q, want git", r.Name())
	}
	if r.Describe() != "wraps the git capability" {
		t.Errorf("Describe = %q, want descriptive text", r.Describe())
	}

	ev, err := r.Gather(context.Background(), ResearchQuery{Question: "summary"}, EvidenceBundle{})
	if err != nil {
		t.Fatalf("Gather: unexpected error: %v", err)
	}
	if ev.Source != "git" {
		t.Errorf("Source = %q, want git", ev.Source)
	}
	if !strings.Contains(ev.Body, "commit a") || !strings.Contains(ev.Body, "commit b") {
		t.Errorf("Body missing stream events: %q", ev.Body)
	}
}

func TestCapabilityResearcherDocEventsBecomeMetaAndRefs(t *testing.T) {
	cap := &fakeCap{
		manifest: capability.Manifest{Name: "git", Description: "git"},
		events: []capability.Event{
			{Kind: capability.EventDoc, Doc: map[string]any{"sha": "abc123", "msg": "fix"}},
			{Kind: capability.EventDoc, Doc: map[string]any{"path": "/tmp/x.go", "msg": "edit"}},
		},
	}
	r := NewCapabilityResearcher(cap)
	ev, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err != nil {
		t.Fatalf("Gather: unexpected error: %v", err)
	}
	if len(ev.Refs) != 2 {
		t.Errorf("Refs = %v, want 2 entries", ev.Refs)
	}
	if got := ev.Refs[0]; got != "abc123" {
		t.Errorf("Refs[0] = %q, want abc123", got)
	}
	if got := ev.Refs[1]; got != "/tmp/x.go" {
		t.Errorf("Refs[1] = %q, want /tmp/x.go", got)
	}
	if _, ok := ev.Meta["doc.1"]; !ok {
		t.Errorf("Meta missing doc.1: %v", ev.Meta)
	}
}

func TestCapabilityResearcherErrorEventsSurfaceFromGather(t *testing.T) {
	want := errors.New("git failed to read HEAD")
	cap := &fakeCap{
		manifest: capability.Manifest{Name: "git", Description: "git"},
		events: []capability.Event{
			{Kind: capability.EventStream, Text: "partial output"},
			{Kind: capability.EventError, Err: want},
		},
	}
	r := NewCapabilityResearcher(cap)
	ev, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err == nil {
		t.Fatal("Gather: expected non-nil error from EventError, got nil")
	}
	if !strings.Contains(err.Error(), "git failed to read HEAD") {
		t.Errorf("error chain missing original message: %v", err)
	}
	// Partial body must still be returned so the loop can use what it got.
	if !strings.Contains(ev.Body, "partial output") {
		t.Errorf("partial body lost on error: %q", ev.Body)
	}
}

func TestCapabilityResearcherInvokeErrorPropagates(t *testing.T) {
	want := errors.New("capability is offline")
	cap := &fakeCap{
		manifest:  capability.Manifest{Name: "git", Description: "git"},
		invokeErr: want,
	}
	r := NewCapabilityResearcher(cap)
	_, err := r.Gather(context.Background(), ResearchQuery{Question: "q"}, EvidenceBundle{})
	if err == nil || !strings.Contains(err.Error(), "capability is offline") {
		t.Errorf("expected wrapped invoke error, got %v", err)
	}
}

func TestRegisterNativesPicksRequestedNamesOnly(t *testing.T) {
	src := capability.NewRegistry()
	for _, name := range []string{"git", "esearch", "observer", "brainrag", "extra"} {
		err := src.Register(&fakeCap{manifest: capability.Manifest{Name: name, Description: name}})
		if err != nil {
			t.Fatalf("seed Register(%s): %v", name, err)
		}
	}

	dst := NewRegistry()
	if err := RegisterNatives(dst, src, DefaultNativeNames...); err != nil {
		t.Fatalf("RegisterNatives: %v", err)
	}
	got := dst.Names()
	want := []string{"brainrag", "esearch", "git", "observer"}
	if len(got) != len(want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRegisterNativesEmptyMeansAll(t *testing.T) {
	src := capability.NewRegistry()
	for _, name := range []string{"a", "b"} {
		_ = src.Register(&fakeCap{manifest: capability.Manifest{Name: name, Description: name}})
	}
	dst := NewRegistry()
	if err := RegisterNatives(dst, src); err != nil {
		t.Fatalf("RegisterNatives: %v", err)
	}
	if len(dst.Names()) != 2 {
		t.Errorf("expected all 2 capabilities registered, got %v", dst.Names())
	}
}

func TestRegisterNativesMissingCapabilityIsAnError(t *testing.T) {
	src := capability.NewRegistry()
	_ = src.Register(&fakeCap{manifest: capability.Manifest{Name: "git", Description: "git"}})
	dst := NewRegistry()
	err := RegisterNatives(dst, src, "git", "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing capability, got nil")
	}
	// The valid one should still have been registered.
	if _, ok := dst.Lookup("git"); !ok {
		t.Errorf("expected valid capability to be registered despite the missing one")
	}
}

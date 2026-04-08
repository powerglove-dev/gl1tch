package research

import (
	"context"
	"errors"
	"testing"
)

// stubResearcher is a minimal Researcher used by the registry tests. It
// reports a fixed Name/Describe and returns an empty Evidence on Gather.
type stubResearcher struct {
	name     string
	describe string
}

func (s stubResearcher) Name() string                                        { return s.name }
func (s stubResearcher) Describe() string                                    { return s.describe }
func (s stubResearcher) Gather(_ context.Context, _ ResearchQuery, _ EvidenceBundle) (Evidence, error) {
	return Evidence{Source: s.name}, nil
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	want := stubResearcher{name: "git", describe: "wraps the git capability"}
	if err := r.Register(want); err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}
	got, ok := r.Lookup("git")
	if !ok {
		t.Fatalf("Lookup(git): expected ok=true")
	}
	if got.Name() != "git" {
		t.Errorf("Lookup(git).Name() = %q, want git", got.Name())
	}
	if got.Describe() != want.Describe() {
		t.Errorf("Lookup(git).Describe() = %q, want %q", got.Describe(), want.Describe())
	}
}

func TestRegistryDuplicateName(t *testing.T) {
	r := NewRegistry()
	first := stubResearcher{name: "git", describe: "first"}
	second := stubResearcher{name: "git", describe: "second"}
	if err := r.Register(first); err != nil {
		t.Fatalf("first Register: unexpected error: %v", err)
	}
	err := r.Register(second)
	if err == nil {
		t.Fatal("second Register: expected duplicate error, got nil")
	}
	if !errors.Is(err, ErrDuplicateResearcher) {
		t.Errorf("expected ErrDuplicateResearcher, got %v", err)
	}
	// The original registration must still be present.
	got, _ := r.Lookup("git")
	if got.Describe() != "first" {
		t.Errorf("after duplicate attempt, Lookup(git).Describe() = %q, want first", got.Describe())
	}
}

func TestRegistryLookupUnknownReturnsFalse(t *testing.T) {
	r := NewRegistry()
	got, ok := r.Lookup("does-not-exist")
	if ok {
		t.Errorf("Lookup(does-not-exist): expected ok=false, got ok=true")
	}
	if got != nil {
		t.Errorf("Lookup(does-not-exist): expected nil researcher, got %v", got)
	}
}

func TestRegistryRegisterRejectsNil(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatal("Register(nil): expected error, got nil")
	}
}

func TestRegistryRegisterRejectsEmptyName(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubResearcher{name: "", describe: "x"}); err == nil {
		t.Fatal("Register(empty name): expected error, got nil")
	}
}

func TestRegistryListAndNamesAreSorted(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"observer", "git", "esearch", "brainrag"} {
		if err := r.Register(stubResearcher{name: n, describe: n}); err != nil {
			t.Fatalf("Register(%s): %v", n, err)
		}
	}
	gotNames := r.Names()
	wantNames := []string{"brainrag", "esearch", "git", "observer"}
	if len(gotNames) != len(wantNames) {
		t.Fatalf("Names: got %d entries, want %d", len(gotNames), len(wantNames))
	}
	for i, n := range wantNames {
		if gotNames[i] != n {
			t.Errorf("Names[%d] = %q, want %q", i, gotNames[i], n)
		}
	}
	gotList := r.List()
	if len(gotList) != len(wantNames) {
		t.Fatalf("List: got %d entries, want %d", len(gotList), len(wantNames))
	}
	for i, n := range wantNames {
		if gotList[i].Name() != n {
			t.Errorf("List[%d].Name() = %q, want %q", i, gotList[i].Name(), n)
		}
	}
}

func TestEvidenceBundleSourcesUnique(t *testing.T) {
	b := &EvidenceBundle{}
	b.Add(Evidence{Source: "git", Title: "a"})
	b.Add(Evidence{Source: "git", Title: "b"})
	b.Add(Evidence{Source: "esearch", Title: "c"})
	b.Add(Evidence{Source: "git", Title: "d"})
	b.Add(Evidence{Source: "observer", Title: "e"})
	got := b.Sources()
	want := []string{"git", "esearch", "observer"}
	if len(got) != len(want) {
		t.Fatalf("Sources: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Sources[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBudgetDefaults(t *testing.T) {
	b := DefaultBudget()
	if b.MaxIterations <= 0 || b.MaxLocalTokens <= 0 || b.MaxWallclock <= 0 {
		t.Errorf("DefaultBudget: expected positive caps, got %+v", b)
	}
	if b.MaxPaidTokens != 0 {
		t.Errorf("DefaultBudget: MaxPaidTokens = %d, want 0 (escalation off by default)", b.MaxPaidTokens)
	}
}

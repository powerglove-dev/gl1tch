package switchboard

import (
	"testing"
)

func TestFuzzyScore_EmptyQuery(t *testing.T) {
	if fuzzyScore("/Users/stokes/Projects/foo", "") != 1 {
		t.Error("empty query should return 1 (always matches)")
	}
}

func TestFuzzyScore_NoMatch(t *testing.T) {
	if fuzzyScore("/Users/stokes/Projects/foo", "zzzzz") != 0 {
		t.Error("non-matching query should return 0")
	}
}

func TestFuzzyScore_ContiguousBaseNameMatch(t *testing.T) {
	score := fuzzyScore("/Users/stokes/Projects/orcai", "orcai")
	if score < 1000 {
		t.Errorf("contiguous base name match should score ≥1000, got %d", score)
	}
}

func TestFuzzyScore_BaseNameBeatsFull(t *testing.T) {
	// "orcai" in base name should score higher than "orcai" buried in a long path.
	scoreBase := fuzzyScore("/Users/stokes/Projects/orcai", "orcai")
	scoreFull := fuzzyScore("/orcai/deep/nested/something-else", "orcai")
	if scoreBase <= scoreFull {
		t.Errorf("basename match (%d) should beat full-path early match (%d)", scoreBase, scoreFull)
	}
}

func TestFuzzyScore_FuzzyOrderMatch(t *testing.T) {
	// "pjts" should fuzzy-match "/home/user/projects" (p-r-o-j-e-c-t-s has p,j,t,s in order)
	score := fuzzyScore("/home/user/projects", "pjts")
	if score == 0 {
		t.Error("fuzzy order match should return non-zero score")
	}
}

func TestDirPickerModel_ApplyFilter_Empty(t *testing.T) {
	m := NewDirPickerModel()
	m.allDirs = []string{"/a", "/b", "/c"}
	m.walking = false
	m.applyFilter()
	if len(m.shown) != 3 {
		t.Errorf("empty query should show all %d dirs, got %d", 3, len(m.shown))
	}
}

func TestDirPickerModel_ApplyFilter_Caps50(t *testing.T) {
	m := NewDirPickerModel()
	dirs := make([]string, 100)
	for i := range 100 {
		dirs[i] = "/dir/path/" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	m.allDirs = dirs
	m.walking = false
	m.applyFilter()
	if len(m.shown) > dirPickerMaxResults {
		t.Errorf("shown should be capped at %d, got %d", dirPickerMaxResults, len(m.shown))
	}
}

func TestDirPickerModel_ApplyFilter_FiltersCorrectly(t *testing.T) {
	m := NewDirPickerModel()
	m.allDirs = []string{
		"/home/user/orcai",
		"/home/user/documents",
		"/home/user/go",
	}
	m.walking = false
	m.input.SetValue("orcai")
	m.applyFilter()
	if len(m.shown) != 1 || m.shown[0] != "/home/user/orcai" {
		t.Errorf("filter 'orcai' should match only orcai, got %v", m.shown)
	}
}

func TestDirPickerModel_CursorResetOnFilterChange(t *testing.T) {
	m := NewDirPickerModel()
	m.allDirs = []string{"/a/foo", "/b/bar", "/c/baz"}
	m.walking = false
	m.cursor = 2
	m.input.SetValue("fo")
	m.applyFilter()
	if m.cursor != 0 {
		t.Errorf("cursor should reset to 0 after filter change, got %d", m.cursor)
	}
}

package console

import (
	"testing"
)

// TestDirPickerShim verifies that the switchboard shim types and constructors
// correctly delegate to the modal package implementation.
func TestDirPickerShim_NewDirPickerModel(t *testing.T) {
	m := NewDirPickerModel()
	// DirPickerModel is a type alias for modal.DirPickerModel; just verify it constructs.
	_ = m
}

func TestDirPickerShim_TypeAliases(t *testing.T) {
	// Verify DirSelectedMsg and DirCancelledMsg are usable as switchboard types.
	sel := DirSelectedMsg{Path: "/tmp/foo"}
	if sel.Path != "/tmp/foo" {
		t.Errorf("DirSelectedMsg.Path = %q, want %q", sel.Path, "/tmp/foo")
	}
	var _ DirCancelledMsg
}

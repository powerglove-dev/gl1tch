package console

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/powerglove-dev/gl1tch/internal/modal"
)

// DirPickerModel, DirSelectedMsg, DirCancelledMsg, dirWalkResultMsg are now
// provided by internal/modal. These type aliases maintain backward compatibility.
type DirPickerModel = modal.DirPickerModel
type DirSelectedMsg = modal.DirSelectedMsg
type DirCancelledMsg = modal.DirCancelledMsg

// dirWalkResultMsg is an alias for modal.DirWalkResultMsg so that existing
// switchboard code can still type-switch on it.
type dirWalkResultMsg = modal.DirWalkResultMsg

// NewDirPickerModel delegates to modal.NewDirPickerModel.
func NewDirPickerModel() DirPickerModel { return modal.NewDirPickerModel() }

// DirPickerInit delegates to modal.DirPickerInit.
func DirPickerInit() tea.Cmd { return modal.DirPickerInit() }

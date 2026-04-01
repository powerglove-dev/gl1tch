package modal_test

import (
	"encoding/json"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/modal"
	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/styles"
)

func testRun(kind string) store.Run {
	meta, _ := json.Marshal(map[string]string{"model": "test-provider/model-a", "cwd": "/tmp"})
	r := store.Run{
		ID:       1,
		Kind:     kind,
		Name:     "test-run",
		Metadata: string(meta),
	}
	if kind == "agent" {
		r.Steps = []store.StepRecord{{ID: "step1", Prompt: "hello world"}}
	}
	return r
}

func rerunTestProviders() []picker.ProviderDef {
	return []picker.ProviderDef{{
		ID:    "test-provider",
		Label: "Test Provider",
		Models: []picker.ModelOption{
			{ID: "model-a", Label: "Model A"},
			{ID: "model-b", Label: "Model B"},
		},
	}}
}

func TestNewRerunModal_SeedsPickerFromMetadata(t *testing.T) {
	m := modal.NewRerunModal(testRun("agent"), rerunTestProviders(), "/tmp")
	if got := m.Run().Name; got != "test-run" {
		t.Fatalf("unexpected run name: %q", got)
	}
}

func TestNewRerunModal_AgentPreFillsTextarea(t *testing.T) {
	m := modal.NewRerunModal(testRun("agent"), rerunTestProviders(), "/tmp")
	pal := styles.ANSIPalette{Border: "\x1b[36m", Accent: "\x1b[35m", FG: "\x1b[97m", Dim: "\x1b[2m"}
	view := m.ViewBox(60, 24, pal)
	if view == "" {
		t.Fatal("ViewBox returned empty string")
	}
}

func TestRerunModal_EscEmitsCancelledMsg(t *testing.T) {
	m := modal.NewRerunModal(testRun("agent"), rerunTestProviders(), "/tmp")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command on esc")
	}
	msg := cmd()
	if _, ok := msg.(modal.RerunCancelledMsg); !ok {
		t.Fatalf("expected RerunCancelledMsg, got %T", msg)
	}
}

func TestRerunModal_CtrlREmitsConfirmedMsg(t *testing.T) {
	m := modal.NewRerunModal(testRun("agent"), rerunTestProviders(), "/tmp")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatal("expected a command on ctrl+r")
	}
	msg := cmd()
	confirmed, ok := msg.(modal.RerunConfirmedMsg)
	if !ok {
		t.Fatalf("expected RerunConfirmedMsg, got %T", msg)
	}
	if confirmed.Run.Name != "test-run" {
		t.Fatalf("unexpected run name in confirmed msg: %q", confirmed.Run.Name)
	}
}

func TestRerunModal_TabCyclesFocus(t *testing.T) {
	m := modal.NewRerunModal(testRun("agent"), rerunTestProviders(), "/tmp")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})  // context → cwd
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab}) // cwd → provider
	_, cmd := m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter in picker focus")
	}
	msg := cmd()
	if _, ok := msg.(modal.RerunConfirmedMsg); !ok {
		t.Fatalf("expected RerunConfirmedMsg after tab+enter, got %T", msg)
	}
}

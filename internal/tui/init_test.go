package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInit_IKeyReturnsExecCmd(t *testing.T) {
	m := testModel()
	m.focus = FocusLeft

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	model := result.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil cmd from i key")
	}

	// tea.ExecProcess does NOT go through cmdStartMsg, so isLoading stays false.
	// This is the behavioral difference from the old runTfCmd approach.
	if model.isLoading {
		t.Error("i key should use tea.ExecProcess (no loading state), not runTfCmd")
	}
}

func TestInit_IKeyBlockedWhenBusy(t *testing.T) {
	m := testModel()
	m.focus = FocusLeft
	m.isLoading = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	model := result.(Model)

	if !strings.Contains(model.statusMsg, "Command in progress") {
		t.Errorf("expected busy message, got %q", model.statusMsg)
	}
}

func TestInit_FinishedMsgReloadsData(t *testing.T) {
	m := testModel()

	result, cmd := m.Update(initFinishedMsg{err: nil})
	model := result.(Model)

	// Should trigger a data reload
	if cmd == nil {
		t.Error("expected a data reload cmd after init finishes")
	}

	// Should show success status
	if !strings.Contains(model.statusMsg, "Init complete") {
		t.Errorf("expected success status, got %q", model.statusMsg)
	}
}

func TestInit_FinishedMsgWithError(t *testing.T) {
	m := testModel()

	result, cmd := m.Update(initFinishedMsg{err: errFake("init failed")})
	model := result.(Model)

	// Should still reload data (init may have partially succeeded)
	if cmd == nil {
		t.Error("expected a data reload cmd even after init error")
	}

	// Should show error status
	if !strings.Contains(model.statusMsg, "Init failed") {
		t.Errorf("expected error status, got %q", model.statusMsg)
	}
}

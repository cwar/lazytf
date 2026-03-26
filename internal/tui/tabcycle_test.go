package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"

	"github.com/cwar/lazytf/internal/terraform"
)

func tabTestModel(panel PanelID) Model {
	return Model{
		width:       120,
		height:      30,
		panels:      makePanels(),
		activePanel: panel,
		focus:       FocusLeft,
		runner:      terraform.NewRunner("/tmp"),
	}
}

// --- Tab cycles panels forward ---

func TestTab_CyclesForwardFromFiles(t *testing.T) {
	m := tabTestModel(PanelFiles)
	got := sendSpecialKey(m, tea.KeyTab)
	if got.activePanel != PanelResources {
		t.Fatalf("expected Resources panel, got %d", got.activePanel)
	}
	if got.focus != FocusLeft {
		t.Fatal("expected focus to stay on left")
	}
}

func TestTab_CyclesForwardWraps(t *testing.T) {
	m := tabTestModel(PanelHistory)
	got := sendSpecialKey(m, tea.KeyTab)
	if got.activePanel != PanelStatus {
		t.Fatalf("expected Status panel (wrap), got %d", got.activePanel)
	}
}

func TestTab_FromRightFocus_CyclesAndSetsFocusLeft(t *testing.T) {
	m := tabTestModel(PanelResources)
	m.focus = FocusRight
	got := sendSpecialKey(m, tea.KeyTab)
	if got.activePanel != PanelWorkspaces {
		t.Fatalf("expected Workspaces panel, got %d", got.activePanel)
	}
	if got.focus != FocusLeft {
		t.Fatal("expected focus to move to left after Tab")
	}
}

// --- Shift+Tab cycles panels backward ---

func TestShiftTab_CyclesBackward(t *testing.T) {
	m := tabTestModel(PanelResources)
	got := sendSpecialKey(m, tea.KeyShiftTab)
	if got.activePanel != PanelFiles {
		t.Fatalf("expected Files panel, got %d", got.activePanel)
	}
}

func TestShiftTab_WrapsFromStatus(t *testing.T) {
	m := tabTestModel(PanelStatus)
	got := sendSpecialKey(m, tea.KeyShiftTab)
	if got.activePanel != PanelHistory {
		t.Fatalf("expected History panel (wrap), got %d", got.activePanel)
	}
}

// --- Tab still works during busy (navigation, not blocked) ---

func TestTab_AllowedWhileBusy(t *testing.T) {
	m := baseBusyModel()
	m.activePanel = PanelFiles
	m.runner = terraform.NewRunner("/tmp")
	got := sendSpecialKey(m, tea.KeyTab)
	if got.activePanel != PanelResources {
		t.Fatalf("expected Tab to cycle panels while busy, got panel %d", got.activePanel)
	}
}

func TestShiftTab_AllowedWhileBusy(t *testing.T) {
	m := baseBusyModel()
	m.activePanel = PanelResources
	m.runner = terraform.NewRunner("/tmp")
	got := sendSpecialKey(m, tea.KeyShiftTab)
	if got.activePanel != PanelFiles {
		t.Fatalf("expected Shift+Tab to cycle panels while busy, got panel %d", got.activePanel)
	}
}

// --- Plan review: Tab still toggles focus view (not panel cycle) ---

func TestF_InPlanReview_TogglesFocus(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = false
	got := sendKey(m, "f")
	if !got.planView.focusView {
		t.Fatal("expected f in plan review to toggle focus view")
	}
	// Panel should not change
	if got.activePanel != m.activePanel {
		t.Fatal("expected activePanel unchanged during plan review")
	}
}

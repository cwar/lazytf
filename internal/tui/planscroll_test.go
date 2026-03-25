package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

// helper to send a key to the model and return the updated model
func sendKey(m Model, key string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(Model)
}

func sendSpecialKey(m Model, keyType tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: keyType})
	return updated.(Model)
}

func basePlanReviewModel() Model {
	// 50 lines of plan output
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	return Model{
		width:           120,
		height:          30, // visH = 30 - 6 = 24
		planReview:      true,
		pendingPlanFile: "/tmp/test.tfplan",
		detailLines:     lines,
		planChanges: []planChange{
			{Address: "aws_instance.a", Action: "create", Line: 0, EndLine: 15},
			{Address: "aws_instance.b", Action: "update", Line: 15, EndLine: 45},
			{Address: "aws_instance.c", Action: "destroy", Line: 45, EndLine: 50},
		},
		detailScroll: 0,
		panels:       makePanels(), // reuse helper from workspace_switch_test
	}
}

// --- Focus mode: j/k scroll within the focused resource block ---

func TestFocusMode_JScrollsDown(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.planView.changeCur = 1 // resource 1 has 30 lines (15..45), overflows viewport
	m.detailScroll = 0

	m = sendKey(m, "j")

	if m.detailScroll != 1 {
		t.Errorf("detailScroll = %d, want 1", m.detailScroll)
	}
	// Should stay on same resource
	if m.planView.changeCur != 1 {
		t.Errorf("changeCur = %d, want 1 (should not change resource)", m.planView.changeCur)
	}
}

func TestFocusMode_KScrollsUp(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.detailScroll = 5

	m = sendKey(m, "k")

	if m.detailScroll != 4 {
		t.Errorf("detailScroll = %d, want 4", m.detailScroll)
	}
	if m.planView.changeCur != 0 {
		t.Errorf("changeCur = %d, want 0", m.planView.changeCur)
	}
}

func TestFocusMode_KDoesNotScrollBelowZero(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.detailScroll = 0

	m = sendKey(m, "k")

	if m.detailScroll != 0 {
		t.Errorf("detailScroll = %d, want 0", m.detailScroll)
	}
}

func TestFocusMode_JClampsToMax(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	// Resource 0 has 15 lines (0..15), visH = 24, so max scroll = 0 (fits)
	// But resource 1 has 30 lines (15..45), max scroll = 30 - 24 = 6
	m.planView.changeCur = 1
	m.detailScroll = 6 // already at max

	m = sendKey(m, "j")

	if m.detailScroll != 6 {
		t.Errorf("detailScroll = %d, want 6 (should clamp at max)", m.detailScroll)
	}
}

func TestFocusMode_DownArrowScrolls(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.planView.changeCur = 1 // resource with enough lines to scroll
	m.detailScroll = 0

	m = sendSpecialKey(m, tea.KeyDown)

	if m.detailScroll != 1 {
		t.Errorf("detailScroll = %d, want 1", m.detailScroll)
	}
	if m.planView.changeCur != 1 {
		t.Errorf("changeCur = %d, want 1", m.planView.changeCur)
	}
}

func TestFocusMode_UpArrowScrolls(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.detailScroll = 3

	m = sendSpecialKey(m, tea.KeyUp)

	if m.detailScroll != 2 {
		t.Errorf("detailScroll = %d, want 2", m.detailScroll)
	}
}

// --- Focus mode: n/N still navigate between resources ---

func TestFocusMode_NNavigatesNext(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.planView.changeCur = 0
	m.detailScroll = 5

	m = sendKey(m, "n")

	if m.planView.changeCur != 1 {
		t.Errorf("changeCur = %d, want 1", m.planView.changeCur)
	}
	if m.detailScroll != 0 {
		t.Errorf("detailScroll = %d, want 0 (should reset on resource change)", m.detailScroll)
	}
}

func TestFocusMode_ShiftNNavigatesPrev(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.planView.changeCur = 1
	m.detailScroll = 3

	m = sendKey(m, "N")

	if m.planView.changeCur != 0 {
		t.Errorf("changeCur = %d, want 0", m.planView.changeCur)
	}
	if m.detailScroll != 0 {
		t.Errorf("detailScroll = %d, want 0", m.detailScroll)
	}
}

func TestFocusMode_NWrapsAround(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.focusView = true
	m.planView.changeCur = 2 // last resource

	m = sendKey(m, "n")

	if m.planView.changeCur != 0 {
		t.Errorf("changeCur = %d, want 0 (should wrap)", m.planView.changeCur)
	}
}

// --- Non-focus mode: j/k navigate between resources (unchanged behavior) ---

func TestNonFocus_JNavigatesNext(t *testing.T) {
	m := basePlanReviewModel()
	// focusView and changeCur default to zero values (off, 0)

	m = sendKey(m, "j")

	if m.planView.changeCur != 1 {
		t.Errorf("changeCur = %d, want 1", m.planView.changeCur)
	}
	if m.detailScroll != 15 {
		t.Errorf("detailScroll = %d, want 15 (resource 1 starts at line 15)", m.detailScroll)
	}
}

func TestNonFocus_KNavigatesPrev(t *testing.T) {
	m := basePlanReviewModel()
	m.planView.changeCur = 1

	m = sendKey(m, "k")

	if m.planView.changeCur != 0 {
		t.Errorf("changeCur = %d, want 0", m.planView.changeCur)
	}
	if m.detailScroll != 0 {
		t.Errorf("detailScroll = %d, want 0 (resource 0 starts at line 0)", m.detailScroll)
	}
}

func TestNonFocus_NNavigatesNext(t *testing.T) {
	m := basePlanReviewModel()
	// focusView defaults to false, changeCur defaults to 0

	m = sendKey(m, "n")

	if m.planView.changeCur != 1 {
		t.Errorf("changeCur = %d, want 1", m.planView.changeCur)
	}
}

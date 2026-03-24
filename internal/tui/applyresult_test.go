package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/terraform"
)

// testModel creates a minimal Model suitable for tests that involve
// dataLoadedMsg (which triggers rebuildAllPanels and needs a runner).
func testModel() Model {
	return Model{
		width:  120,
		height: 40,
		panels: makePanels(),
		runner: terraform.NewRunner("/tmp"),
	}
}

// TestApplyResult_StaysVisibleAfterDataReload verifies that when an apply
// completes, the output stays visible even when dataLoadedMsg arrives
// (which normally calls onSelectionChanged and overwrites the detail pane).
func TestApplyResult_StaysVisibleAfterDataReload(t *testing.T) {
	m := testModel()

	// Simulate a completed Apply (streamed output)
	doneMsg := cmdDoneMsg{
		title:    "Apply",
		err:      nil,
		streamed: true,
	}
	m.detailLines = []string{"Apply complete!", "Resources: 1 added"}
	m.highlightedLines = []string{"Apply complete!", "Resources: 1 added"}
	m.isLoading = true

	result, _ := m.Update(doneMsg)
	updated := result.(Model)

	if !updated.applyResult {
		t.Fatal("applyResult should be true after Apply completes")
	}

	// Now simulate dataLoadedMsg arriving (the data reload)
	dataMsg := dataLoadedMsg{}
	result2, _ := updated.Update(dataMsg)
	updated2 := result2.(Model)

	// The detail pane should still show the apply output, NOT be overwritten
	if !updated2.applyResult {
		t.Fatal("applyResult should still be true after dataLoadedMsg")
	}
	if len(updated2.detailLines) == 0 {
		t.Fatal("detailLines should still contain apply output")
	}
	if updated2.detailLines[0] != "Apply complete!" {
		t.Errorf("detail pane was overwritten: got %q", updated2.detailLines[0])
	}
}

// TestApplyResult_DestroyAlsoStays verifies destroy output is also preserved.
func TestApplyResult_DestroyAlsoStays(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.detailLines = []string{"Destroy complete! Resources: 2 destroyed."}
	m.highlightedLines = []string{"Destroy complete! Resources: 2 destroyed."}

	doneMsg := cmdDoneMsg{title: "Destroy", err: nil, streamed: true}
	result, _ := m.Update(doneMsg)
	updated := result.(Model)

	if !updated.applyResult {
		t.Fatal("applyResult should be true after Destroy completes")
	}
}

// TestApplyResult_DismissWithEsc verifies pressing esc clears the apply result.
func TestApplyResult_DismissWithEsc(t *testing.T) {
	m := testModel()
	m.applyResult = true
	m.detailLines = []string{"Apply complete!"}
	m.detailTitle = "Apply"
	m.focus = FocusRight

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(Model)

	if updated.applyResult {
		t.Fatal("applyResult should be false after esc")
	}
}

// TestApplyResult_DismissWithQ verifies pressing q clears the apply result.
func TestApplyResult_DismissWithQ(t *testing.T) {
	m := testModel()
	m.applyResult = true
	m.detailLines = []string{"Apply complete!"}
	m.detailTitle = "Apply"
	m.focus = FocusRight

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated := result.(Model)

	if updated.applyResult {
		t.Fatal("applyResult should be false after q")
	}
}

// TestApplyResult_NotSetForPlan verifies that plan commands don't set applyResult
// (plans enter planReview mode instead).
func TestApplyResult_NotSetForPlan(t *testing.T) {
	m := testModel()
	m.isLoading = true

	doneMsg := cmdDoneMsg{title: "Plan", err: nil, streamed: true}
	m.detailLines = []string{"No changes."}
	result, _ := m.Update(doneMsg)
	updated := result.(Model)

	if updated.applyResult {
		t.Fatal("applyResult should NOT be set for Plan commands")
	}
}

// TestApplyResult_NotSetOnError verifies that a failed apply doesn't pin the output.
func TestApplyResult_NotSetOnError(t *testing.T) {
	m := testModel()
	m.isLoading = true

	doneMsg := cmdDoneMsg{
		title:    "Apply",
		err:      errFake("apply failed"),
		streamed: true,
	}
	m.detailLines = []string{"Error: apply failed"}
	result, _ := m.Update(doneMsg)
	updated := result.(Model)

	// Even errors should be pinned — the user needs to see what went wrong
	if !updated.applyResult {
		t.Fatal("applyResult should be true even on error, so user can see what happened")
	}
}

// TestApplyResult_ClearedOnNewCommand verifies that starting a new command
// clears the applyResult state.
func TestApplyResult_ClearedOnNewCommand(t *testing.T) {
	m := testModel()
	m.applyResult = true

	startMsg := cmdStartMsg{title: "Plan"}
	result, _ := m.Update(startMsg)
	updated := result.(Model)

	if updated.applyResult {
		t.Fatal("applyResult should be cleared when a new command starts")
	}
}

// TestApplyResult_StatusBarHint verifies the status message tells the user
// how to dismiss the apply result.
func TestApplyResult_StatusBarHint(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.detailLines = []string{"Apply complete!"}
	m.highlightedLines = []string{"Apply complete!"}

	doneMsg := cmdDoneMsg{title: "Apply", err: nil, streamed: true}
	result, _ := m.Update(doneMsg)
	updated := result.(Model)

	// Status should mention pressing esc to dismiss
	if !strings.Contains(updated.statusMsg, "esc") {
		t.Errorf("status should tell user to press esc, got: %q", updated.statusMsg)
	}
}

// TestApplyResult_ScrollingWorks verifies j/k scrolling still works while
// viewing the apply result.
func TestApplyResult_ScrollingWorks(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	m := testModel()
	m.applyResult = true
	m.detailLines = lines
	m.highlightedLines = lines
	m.detailTitle = "Apply"
	m.focus = FocusRight
	m.detailScroll = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	updated := result.(Model)

	if updated.detailScroll == 0 {
		t.Fatal("j should scroll down while viewing apply result")
	}
}

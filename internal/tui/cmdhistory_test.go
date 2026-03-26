package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ─── cmdHistory unit tests ───────────────────────────────

func TestCmdHistory_PushAndGet(t *testing.T) {
	var h cmdHistory
	h.push(cmdRecord{title: "Plan", workspace: "dev"})
	h.push(cmdRecord{title: "Apply", workspace: "dev"})

	if h.len() != 2 {
		t.Fatalf("expected 2 entries, got %d", h.len())
	}
	// Most recent first
	if h.get(0).title != "Apply" {
		t.Errorf("entry 0 should be Apply, got %q", h.get(0).title)
	}
	if h.get(1).title != "Plan" {
		t.Errorf("entry 1 should be Plan, got %q", h.get(1).title)
	}
}

func TestCmdHistory_MaxEntries(t *testing.T) {
	var h cmdHistory
	for i := 0; i < maxHistoryEntries+10; i++ {
		h.push(cmdRecord{title: "cmd"})
	}
	if h.len() != maxHistoryEntries {
		t.Fatalf("expected %d entries (capped), got %d", maxHistoryEntries, h.len())
	}
}

func TestCmdHistory_GetOutOfBounds(t *testing.T) {
	var h cmdHistory
	if h.get(0) != nil {
		t.Fatal("get on empty history should return nil")
	}
	h.push(cmdRecord{title: "Plan"})
	if h.get(-1) != nil {
		t.Fatal("get(-1) should return nil")
	}
	if h.get(5) != nil {
		t.Fatal("get(5) should return nil")
	}
}

// ─── Integration tests: recording history ────────────────

func TestCmdHistory_RecordedOnCmdDone(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.workspace = "dev-gew4"
	m.streamLines = []string{"Refreshing...", "Plan: 1 to add"}
	m.streamHLLines = []string{"Refreshing...", "Plan: 1 to add"}

	doneMsg := cmdDoneMsg{title: "Plan", err: nil, streamed: true}
	result, _ := m.Update(doneMsg)
	m = result.(Model)

	if m.history.len() != 1 {
		t.Fatalf("expected 1 history entry, got %d", m.history.len())
	}
	rec := m.history.get(0)
	if rec.title != "Plan" {
		t.Errorf("title = %q, want Plan", rec.title)
	}
	if rec.workspace != "dev-gew4" {
		t.Errorf("workspace = %q, want dev-gew4", rec.workspace)
	}
	if rec.failed {
		t.Error("should not be marked as failed")
	}
	if len(rec.lines) != 2 {
		t.Errorf("expected 2 output lines, got %d", len(rec.lines))
	}
}

func TestCmdHistory_RecordedOnError(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.workspace = "prod-gew4"
	m.streamLines = []string{"Error: provider not configured"}
	m.streamHLLines = []string{"Error: provider not configured"}

	doneMsg := cmdDoneMsg{title: "Plan", err: errFake("plan failed"), streamed: true}
	result, _ := m.Update(doneMsg)
	m = result.(Model)

	if m.history.len() != 1 {
		t.Fatalf("expected 1 history entry, got %d", m.history.len())
	}
	rec := m.history.get(0)
	if !rec.failed {
		t.Error("should be marked as failed")
	}
}

func TestCmdHistory_NonStreamedRecorded(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.workspace = "dev"

	doneMsg := cmdDoneMsg{
		title:    "Validate",
		output:   "Success! The configuration is valid.",
		err:      nil,
		streamed: false,
	}
	result, _ := m.Update(doneMsg)
	m = result.(Model)

	if m.history.len() != 1 {
		t.Fatalf("expected 1 history entry, got %d", m.history.len())
	}
	rec := m.history.get(0)
	if rec.title != "Validate" {
		t.Errorf("title = %q", rec.title)
	}
	if len(rec.lines) == 0 {
		t.Error("lines should not be empty")
	}
}

func TestCmdHistory_MultipleCommandsOrdered(t *testing.T) {
	m := testModel()
	m.workspace = "dev"

	// Run two commands
	m.isLoading = true
	m.streamLines = []string{"plan output"}
	m.streamHLLines = []string{"plan output"}
	result, _ := m.Update(cmdDoneMsg{title: "Plan", streamed: true})
	m = result.(Model)

	m.isLoading = true
	m.streamLines = []string{"apply output"}
	m.streamHLLines = []string{"apply output"}
	result, _ = m.Update(cmdDoneMsg{title: "Apply", streamed: true})
	m = result.(Model)

	if m.history.len() != 2 {
		t.Fatalf("expected 2, got %d", m.history.len())
	}
	if m.history.get(0).title != "Apply" {
		t.Error("most recent should be Apply")
	}
	if m.history.get(1).title != "Plan" {
		t.Error("second should be Plan")
	}
}

// ─── History panel tests ─────────────────────────────────

func TestHistoryPanel_ShowsEntries(t *testing.T) {
	m := testModel()
	m.workspace = "dev"

	m.history.push(cmdRecord{
		title: "Plan", workspace: "dev", failed: false,
		timestamp: time.Now().Add(-2 * time.Minute),
		lines:     []string{"Plan: 1 to add"},
	})
	m.history.push(cmdRecord{
		title: "Apply", workspace: "dev", failed: true,
		timestamp: time.Now().Add(-1 * time.Minute),
		lines:     []string{"Error: apply failed"},
	})

	m.rebuildHistoryPanel()
	panel := m.panels[PanelHistory]

	if len(panel.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(panel.Items))
	}
	if !strings.Contains(panel.Items[0].Label, "Apply") {
		t.Errorf("first item should be Apply, got %q", panel.Items[0].Label)
	}
	if panel.Items[0].Icon != "✗" {
		t.Errorf("failed item should have ✗ icon, got %q", panel.Items[0].Icon)
	}
	if panel.Items[1].Icon != "✓" {
		t.Errorf("success item should have ✓ icon, got %q", panel.Items[1].Icon)
	}
}

func TestHistoryPanel_SelectShowsOutput(t *testing.T) {
	m := testModel()
	m.history.push(cmdRecord{
		title:     "Plan",
		workspace: "dev",
		lines:     []string{"Plan output line 1", "Plan output line 2"},
		hlLines:   []string{"Plan output line 1", "Plan output line 2"},
	})
	m.rebuildHistoryPanel()
	m.activePanel = PanelHistory
	m.panels[PanelHistory].Cursor = 0

	m.onSelectionChanged()

	if len(m.detailLines) != 2 {
		t.Fatalf("expected 2 detail lines, got %d", len(m.detailLines))
	}
	if m.detailLines[0] != "Plan output line 1" {
		t.Errorf("detail line = %q", m.detailLines[0])
	}
}

func TestHistoryPanel_LKeyJumpsToPanel(t *testing.T) {
	m := testModel()
	m.activePanel = PanelFiles

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = result.(Model)

	if m.activePanel != PanelHistory {
		t.Fatalf("l should jump to history panel, got panel %d", m.activePanel)
	}
}

func TestHistoryPanel_RebuildOnCmdDone(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.workspace = "dev"
	m.streamLines = []string{"plan output"}
	m.streamHLLines = []string{"plan output"}

	result, _ := m.Update(cmdDoneMsg{title: "Plan", streamed: true})
	m = result.(Model)

	panel := m.panels[PanelHistory]
	if len(panel.Items) != 1 {
		t.Fatalf("history panel should have 1 item after command, got %d", len(panel.Items))
	}
}

// ─── Plan changes captured in history ────────────────────

func TestCmdHistory_PlanChanges(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.workspace = "dev"
	m.streamLines = []string{
		"Terraform will perform the following actions:",
		"",
		"  # aws_instance.web will be created",
		"  + resource \"aws_instance\" \"web\" {",
		"      + ami = \"ami-123\"",
		"    }",
	}
	m.streamHLLines = m.streamLines

	result, _ := m.Update(cmdDoneMsg{title: "Plan", streamed: true})
	m = result.(Model)

	rec := m.history.get(0)
	if len(rec.changes) == 0 {
		t.Fatal("history entry should have parsed plan changes")
	}
	if rec.changes[0].Address != "aws_instance.web" {
		t.Errorf("change address = %q", rec.changes[0].Address)
	}
}

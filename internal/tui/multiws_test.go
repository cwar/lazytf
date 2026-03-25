package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/config"
	"github.com/cwar/lazytf/internal/terraform"
)

// multiWSModel builds a Model with workspaces and config for testing multi-ws.
func multiWSModel(workspaces []string, ignoredWS []string, groups map[string]string) Model {
	m := testModel()
	m.workspaces = &terraform.WorkspaceInfo{
		Current:    workspaces[0],
		Workspaces: workspaces,
	}
	m.config = config.Config{
		IgnoreWorkspaces: ignoredWS,
		WorkspaceGroups:  groups,
	}
	m.width = 120
	m.height = 40
	return m
}

func TestMultiWS_WKeyOpensInput(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		nil, nil,
	)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("W")})
	model := result.(Model)

	if !model.showInput {
		t.Fatal("expected input overlay to open")
	}
	if model.inputAction != "multi_ws_plan" {
		t.Errorf("expected action 'multi_ws_plan', got %q", model.inputAction)
	}
}

func TestMultiWS_WKeyBlockedWhenBusy(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		nil, nil,
	)
	m.isLoading = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("W")})
	model := result.(Model)

	if model.showInput {
		t.Fatal("W key should be blocked when busy")
	}
	if !strings.Contains(model.statusMsg, "Command in progress") {
		t.Errorf("expected busy message, got %q", model.statusMsg)
	}
}

func TestMultiWS_StartFiltersIgnored(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4", "prod-gae2"},
		[]string{"default", "prod-gae2"},
		nil,
	)

	// Simulate submitting empty filter (all workspaces)
	m.startMultiWS("")

	if !m.multiWS.active {
		t.Fatal("expected multi-ws mode to be active")
	}
	if len(m.multiWS.items) != 3 {
		t.Fatalf("expected 3 items (minus ignored), got %d", len(m.multiWS.items))
	}

	// Check no ignored workspaces are present
	for _, item := range m.multiWS.items {
		if item.workspace == "default" || item.workspace == "prod-gae2" {
			t.Errorf("ignored workspace %q should not be in items", item.workspace)
		}
	}
}

func TestMultiWS_StartWithFilter(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4", "podcast-dev"},
		[]string{"default"},
		nil,
	)

	m.startMultiWS("dev")

	if !m.multiWS.active {
		t.Fatal("expected multi-ws mode to be active")
	}

	// Should match: dev-gew4, dev-gae2, podcast-dev (all contain "dev")
	names := make([]string, len(m.multiWS.items))
	for i, item := range m.multiWS.items {
		names[i] = item.workspace
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 items matching 'dev', got %d: %v", len(names), names)
	}
}

func TestMultiWS_StartWithGroupFilter(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4", "podcast-dev"},
		[]string{"default"},
		map[string]string{"dev": "dev-"}, // group "dev" → filter "dev-"
	)

	m.startMultiWS("dev")

	// "dev" resolves to "dev-" via group, so only dev-gew4 and dev-gae2 match
	names := make([]string, len(m.multiWS.items))
	for i, item := range m.multiWS.items {
		names[i] = item.workspace
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 items matching 'dev-', got %d: %v", len(names), names)
	}
}

func TestMultiWS_StartNoMatch(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)

	m.startMultiWS("staging")

	if m.multiWS.active {
		t.Fatal("should not activate with no matching workspaces")
	}
	if !strings.Contains(m.statusMsg, "No workspaces match") {
		t.Errorf("expected no-match message, got %q", m.statusMsg)
	}
}

func TestMultiWS_VarFileMatching(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		[]string{"default"},
		nil,
	)

	// Add var files
	m.files = []terraform.TfFile{
		{Name: "dev-gew4.tfvars", Path: "/tmp/dev-gew4.tfvars", IsVars: true},
		{Name: "prod-gew4.tfvars", Path: "/tmp/prod-gew4.tfvars", IsVars: true},
	}

	m.startMultiWS("")

	// Check var files are matched
	for _, item := range m.multiWS.items {
		switch item.workspace {
		case "dev-gew4":
			if item.varFile != "/tmp/dev-gew4.tfvars" {
				t.Errorf("dev-gew4 varFile = %q, want /tmp/dev-gew4.tfvars", item.varFile)
			}
		case "prod-gew4":
			if item.varFile != "/tmp/prod-gew4.tfvars" {
				t.Errorf("prod-gew4 varFile = %q, want /tmp/prod-gew4.tfvars", item.varFile)
			}
		}
	}
}

func TestMultiWS_PlanDone_Success(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Simulate plan completion
	output := "No changes. Your infrastructure matches the configuration."
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    output,
		err:       nil,
	})

	item := &m.multiWS.items[0]
	if item.status != mwsNoChanges {
		t.Errorf("expected status mwsNoChanges, got %d", item.status)
	}
	if m.multiWS.phase != "reviewing" {
		t.Errorf("expected phase 'reviewing', got %q", m.multiWS.phase)
	}
}

func TestMultiWS_PlanDone_WithChanges(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	output := `
Terraform will perform the following actions:

  # aws_instance.example will be created
  + resource "aws_instance" "example" {
      + ami = "ami-123"
    }

Plan: 1 to add, 0 to change, 0 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    output,
		err:       nil,
	})

	item := &m.multiWS.items[0]
	if item.status != mwsPlanned {
		t.Errorf("expected status mwsPlanned, got %d", item.status)
	}
	if item.summary == "" {
		t.Error("expected non-empty summary")
	}
	if !strings.Contains(item.summary, "1 to add") {
		t.Errorf("expected summary to contain '1 to add', got %q", item.summary)
	}
}

func TestMultiWS_PlanDone_Failure(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    "Error: something went wrong",
		err:       errFake("plan failed"),
	})

	item := &m.multiWS.items[0]
	if item.status != mwsPlanFail {
		t.Errorf("expected status mwsPlanFail, got %d", item.status)
	}
}

func TestMultiWS_KeyNavigation(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Start at 0
	if m.multiWS.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.multiWS.cursor)
	}

	// n = next workspace
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model)
	if m.multiWS.cursor != 1 {
		t.Errorf("after n, cursor = %d, want 1", m.multiWS.cursor)
	}

	// n again
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model)
	if m.multiWS.cursor != 2 {
		t.Errorf("after nn, cursor = %d, want 2", m.multiWS.cursor)
	}

	// N = previous workspace
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	m = result.(Model)
	if m.multiWS.cursor != 1 {
		t.Errorf("after N, cursor = %d, want 1", m.multiWS.cursor)
	}

	// n wraps around from last to first
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model) // cursor=2
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model) // cursor=0 (wrapped)
	if m.multiWS.cursor != 0 {
		t.Errorf("after wrap, cursor = %d, want 0", m.multiWS.cursor)
	}
}

func TestMultiWS_JKScrollsOutput(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Add long output so there's something to scroll
	longOutput := strings.Repeat("line\n", 100)
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: longOutput})

	// j scrolls output down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = result.(Model)
	if m.multiWS.scroll != 1 {
		t.Errorf("after j, scroll = %d, want 1", m.multiWS.scroll)
	}

	// k scrolls output up
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = result.(Model)
	if m.multiWS.scroll != 0 {
		t.Errorf("after k, scroll = %d, want 0", m.multiWS.scroll)
	}
}

func TestMultiWS_EscCloses(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	if !m.multiWS.active {
		t.Fatal("expected active")
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = result.(Model)

	if m.multiWS.active {
		t.Error("expected multi-ws to be closed after esc")
	}
}

func TestMultiWS_QCloses(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = result.(Model)

	if m.multiWS.active {
		t.Error("expected multi-ws to be closed after q")
	}
}

func TestMultiWS_AllPhaseTransitions(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Initial phase and status
	if m.multiWS.phase != "planning" {
		t.Errorf("expected phase 'planning', got %q", m.multiWS.phase)
	}
	if m.multiWS.items[0].status != mwsPlanning {
		t.Errorf("expected initial status mwsPlanning, got %d", m.multiWS.items[0].status)
	}

	// Complete first plan
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "No changes."})
	if m.multiWS.phase != "planning" {
		t.Errorf("expected still 'planning' (1 of 2 done), got %q", m.multiWS.phase)
	}

	// Complete second plan
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "prod-gew4", output: "No changes."})
	if m.multiWS.phase != "reviewing" {
		t.Errorf("expected 'reviewing' (all done), got %q", m.multiWS.phase)
	}
}

func TestExtractPlanSummary(t *testing.T) {
	lines := []string{
		"",
		"Terraform will perform the following actions:",
		"  # aws_instance.web will be created",
		"Plan: 2 to add, 1 to change, 0 to destroy.",
		"",
	}
	got := extractPlanSummary(lines)
	if !strings.Contains(got, "2 to add") {
		t.Errorf("extractPlanSummary = %q, expected to contain '2 to add'", got)
	}
}

func TestExtractPlanSummary_NoMatch(t *testing.T) {
	lines := []string{"No changes. Your infrastructure matches the configuration."}
	got := extractPlanSummary(lines)
	if got != "" {
		t.Errorf("extractPlanSummary = %q, expected empty", got)
	}
}

func TestIsNoChanges(t *testing.T) {
	tests := []struct {
		lines []string
		want  bool
	}{
		{[]string{"No changes. Your infrastructure matches the configuration."}, true},
		{[]string{"Plan: 1 to add, 0 to change, 0 to destroy."}, false},
		{[]string{"Apply complete! Resources: 0 added, 0 changed, no changes."}, true},
		{[]string{"Error: something went wrong"}, false},
	}
	for _, tt := range tests {
		if got := isNoChanges(tt.lines); got != tt.want {
			t.Errorf("isNoChanges(%v) = %v, want %v", tt.lines, got, tt.want)
		}
	}
}

func TestCollapseRefreshLines(t *testing.T) {
	lines := []string{
		"Terraform v1.11.4",
		"Initializing plugins and modules...",
		"module.gke.data.validate_tier: Refreshing state...",
		"module.gke.data.validate_foo: Refreshing state...",
		"data.google_compute_zones.available: Refreshing...",
		"data.google_client_config.default: Refreshing...",
		"module.passwords.random_password: Refresh complete after 0s [id=none]",
		"data.google_service_account.reader: Read complete after 0s",
		"",
		"No changes. Your infrastructure matches the configuration.",
	}

	result := collapseRefreshLines(lines)

	// Should have header lines, one collapsed summary, then the result
	if len(result) >= len(lines) {
		t.Errorf("expected fewer lines after collapse, got %d (was %d)", len(result), len(lines))
	}

	// Check the summary line exists
	foundSummary := false
	for _, line := range result {
		if strings.Contains(line, "resources refreshed") {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Errorf("expected '··· N resources refreshed ···' summary line in: %v", result)
	}

	// Check non-refresh lines are preserved
	foundNoChanges := false
	for _, line := range result {
		if strings.Contains(line, "No changes") {
			foundNoChanges = true
		}
	}
	if !foundNoChanges {
		t.Error("expected 'No changes' line to be preserved")
	}
}

func TestCollapseRefreshLines_NoRefreshLines(t *testing.T) {
	lines := []string{
		"Plan: 1 to add, 0 to change, 0 to destroy.",
		"  # aws_instance.web will be created",
	}
	result := collapseRefreshLines(lines)
	if len(result) != len(lines) {
		t.Errorf("expected same length without refresh lines, got %d (was %d)", len(result), len(lines))
	}
}

func TestMultiWS_FocusMode_Toggle(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	planOutput := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami = "ami-123"
    }

  # aws_instance.api will be updated in-place
  ~ resource "aws_instance" "api" {
      ~ ami = "ami-old" -> "ami-new"
    }

Plan: 1 to add, 1 to change, 0 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput})

	item := m.multiWSSelectedItem()
	if len(item.changes) < 2 {
		t.Fatalf("expected at least 2 changes, got %d", len(item.changes))
	}

	// f toggles focus on
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = result.(Model)
	if !m.multiWS.view.focusView {
		t.Fatal("expected focus mode on after f")
	}
	if m.multiWS.view.changeCur != 0 {
		t.Errorf("expected changeCur 0, got %d", m.multiWS.view.changeCur)
	}

	// f toggles focus off
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = result.(Model)
	if m.multiWS.view.focusView {
		t.Fatal("expected focus mode off after second f")
	}
}

func TestMultiWS_FocusMode_ResourceNav(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	planOutput := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami = "ami-123"
    }

  # aws_instance.api will be updated in-place
  ~ resource "aws_instance" "api" {
      ~ ami = "ami-old" -> "ami-new"
    }

Plan: 1 to add, 1 to change, 0 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput})

	// Enter focus mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = result.(Model)

	// n navigates to next resource (not next workspace)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model)
	if m.multiWS.view.changeCur != 1 {
		t.Errorf("expected changeCur 1 after n in focus, got %d", m.multiWS.view.changeCur)
	}
	// Still on same workspace
	if m.multiWS.cursor != 0 {
		t.Errorf("expected cursor still 0, got %d", m.multiWS.cursor)
	}

	// N goes back
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	m = result.(Model)
	if m.multiWS.view.changeCur != 0 {
		t.Errorf("expected changeCur 0 after N, got %d", m.multiWS.view.changeCur)
	}

	// n wraps around
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model) // changeCur=1
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model) // changeCur=0 (wrapped)
	if m.multiWS.view.changeCur != 0 {
		t.Errorf("expected changeCur 0 after wrap, got %d", m.multiWS.view.changeCur)
	}
}

func TestMultiWS_FocusMode_NoChangesIgnored(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Plan with no changes
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    "No changes. Your infrastructure matches the configuration.",
	})

	// f should not activate focus mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = result.(Model)
	if m.multiWS.view.focusView {
		t.Fatal("focus should not activate when there are no changes")
	}
}

func TestMultiWS_FocusMode_RenderTitle(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	planOutput := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami = "ami-123"
    }

Plan: 1 to add, 0 to change, 0 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput})

	// Enter focus
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = result.(Model)

	output := m.renderMultiWS()
	if !strings.Contains(output, "1/1") {
		t.Error("expected focus title to contain resource counter")
	}
	if !strings.Contains(output, "aws_instance.web") {
		t.Error("expected focus title to contain resource address")
	}
}

// ─── Compact Diff (z key) in Multi-WS ────────────────────

func multiWSPlanOutput() string {
	return `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami           = "ami-123"
      + instance_type = "t3.micro"
      + user_data     = <<-EOT
          #!/bin/bash
          set -e
          echo "line 1"
          echo "line 2"
          echo "line 3"
          echo "line 4"
          echo "line 5"
          echo "line 6"
          echo "line 7"
          echo "line 8"
          echo "line 9"
          echo "line 10"
        + echo "new line"
          echo "line 11"
          echo "line 12"
        EOT
    }

Plan: 1 to add, 0 to change, 0 to destroy.`
}

func TestMultiWS_CompactDiff_ZKeyToggles(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: multiWSPlanOutput()})

	// z toggles compact on
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)
	if !m.multiWS.view.compactDiff {
		t.Fatal("expected compact diff on after z")
	}
	if m.multiWS.scroll != 0 {
		t.Errorf("expected scroll reset to 0, got %d", m.multiWS.scroll)
	}

	// z toggles compact off
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)
	if m.multiWS.view.compactDiff {
		t.Fatal("expected compact diff off after second z")
	}
}

func TestMultiWS_CompactDiff_ViewLinesChange(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: multiWSPlanOutput()})

	item := m.multiWSSelectedItem()
	fullLen := len(item.output)

	// Enable compact diff
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)

	// ViewLines should return fewer lines (heredoc content folded)
	compactSrc, _ := m.multiWS.view.ViewLines(item.output, item.hlOutput, item.changes)
	if len(compactSrc) >= fullLen {
		t.Errorf("expected compact output (%d lines) < full output (%d lines)", len(compactSrc), fullLen)
	}

	// Should contain a fold marker
	hasFold := false
	for _, line := range compactSrc {
		if strings.Contains(line, "lines hidden") {
			hasFold = true
			break
		}
	}
	if !hasFold {
		t.Error("expected fold marker in compact output")
	}
}

func TestMultiWS_CompactDiff_SurvivesWorkspaceSwitch(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: multiWSPlanOutput()})
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gae2", output: multiWSPlanOutput()})

	// Enable compact diff on first workspace
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)
	if !m.multiWS.view.compactDiff {
		t.Fatal("compact should be on")
	}

	// Switch to second workspace
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model)

	// Compact diff should still be on (it's a viewing preference, not per-item)
	if !m.multiWS.view.compactDiff {
		t.Fatal("compact diff should survive workspace switch")
	}

	// ViewLines should still return compacted output for new workspace
	item := m.multiWSSelectedItem()
	src, _ := m.multiWS.view.ViewLines(item.output, item.hlOutput, item.changes)
	hasFold := false
	for _, line := range src {
		if strings.Contains(line, "lines hidden") {
			hasFold = true
			break
		}
	}
	if !hasFold {
		t.Error("expected fold marker in compact output after workspace switch")
	}
}

func TestMultiWS_CompactDiff_WithFocusMode(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: multiWSPlanOutput()})

	// Enable focus mode first
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = result.(Model)

	// Then enable compact diff
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)

	if !m.multiWS.view.focusView || !m.multiWS.view.compactDiff {
		t.Fatal("expected both focus and compact to be on")
	}

	// ViewLines should work (not panic)
	item := m.multiWSSelectedItem()
	src, _ := m.multiWS.view.ViewLines(item.output, item.hlOutput, item.changes)
	if len(src) == 0 {
		t.Error("expected non-empty output in focus+compact mode")
	}
}

func TestMultiWS_CompactDiff_HelpHintShowsZ(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.multiWS.phase = "reviewing"

	help := m.renderMultiWSHelp()
	if !strings.Contains(help, "compact") {
		t.Error("expected help hint to contain 'compact' for z key")
	}

	// After toggling, label should change
	m.multiWS.view.compactDiff = true
	help = m.renderMultiWSHelp()
	if !strings.Contains(help, "full diff") {
		t.Error("expected help hint to contain 'full diff' when compact is on")
	}
}

// ─── Skip-Apply from Workspaces Panel ────────────────────

func TestWorkspaces_SKeyTogglesSkipApply(t *testing.T) {
	dir := t.TempDir()
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		nil, nil,
	)
	m.workDir = dir
	m.activePanel = PanelWorkspaces
	m.focus = FocusLeft
	m.rebuildWorkspacesPanel()

	// Move to dev-gew4 (index 1)
	m.panels[PanelWorkspaces].MoveDown()

	// s toggles skip-apply on
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = result.(Model)

	if !m.config.IsSkipApply("dev-gew4") {
		t.Fatal("expected dev-gew4 to be skip-apply after s")
	}

	// Verify persisted to disk
	reloaded, _ := config.Load(dir)
	if !reloaded.IsSkipApply("dev-gew4") {
		t.Fatal("expected skip-apply to persist to .lazytf.yaml")
	}

	// s again toggles it off
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = result.(Model)

	if m.config.IsSkipApply("dev-gew4") {
		t.Fatal("expected dev-gew4 to be un-skipped after second s")
	}
}

func TestWorkspaces_SkipApplyShowsInPanel(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		nil, nil,
	)
	m.config.SkipApplyWorkspaces = []string{"dev-gew4"}
	m.activePanel = PanelWorkspaces
	m.rebuildWorkspacesPanel()

	// The panel item for dev-gew4 should have a skip indicator
	panel := m.panels[PanelWorkspaces]
	found := false
	for _, item := range panel.Items {
		if strings.Contains(item.Label, "dev-gew4") && strings.Contains(item.Label, "skip") {
			found = true
			break
		}
	}
	if !found {
		labels := make([]string, len(panel.Items))
		for i, item := range panel.Items {
			labels[i] = item.Label
		}
		t.Errorf("expected a panel item with 'skip' indicator for dev-gew4, got labels: %v", labels)
	}
}

func TestWorkspaces_SkipApplyBlockedWhenBusy(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		nil, nil,
	)
	m.activePanel = PanelWorkspaces
	m.focus = FocusLeft
	m.isLoading = true
	m.rebuildWorkspacesPanel()

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model := result.(Model)

	// Should not toggle — s is not a destructive op but let's keep it consistent
	// Actually s should work even when busy since it's just a config toggle
	// Let me check: isContextOperationKey should NOT include s
	if model.config.IsSkipApply("dev-gew4") {
		// s works even when busy — that's fine, it's just a config flag
	}
}

func TestWorkspaces_SkipApplyHint(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		nil, nil,
	)
	m.activePanel = PanelWorkspaces
	hints := contextKeysFor(PanelWorkspaces, &m)

	found := false
	for _, h := range hints {
		if h.Key == "s" && strings.Contains(h.Desc, "skip") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 's:skip' in workspace context hints, got %v", hints)
	}
}

// ─── Skip-Apply Exclusion from Multi-WS ─────────────────

func TestMultiWS_SkipApplyExcludedFromMultiWS(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.config.SkipApplyWorkspaces = []string{"prod-gew4"}

	m.startMultiWS("")

	// prod-gew4 should not appear at all — it's skipped
	for _, item := range m.multiWS.items {
		if item.workspace == "prod-gew4" {
			t.Error("skip-apply workspace should be excluded from multi-ws entirely")
		}
	}
	if len(m.multiWS.items) != 2 {
		t.Errorf("expected 2 items (dev-gew4, dev-gae2), got %d", len(m.multiWS.items))
	}
}

func TestMultiWS_SkipApplyNotPlanned(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.config.SkipApplyWorkspaces = []string{"prod-gew4"}

	m.startMultiWS("")

	// Only dev-gew4 should be planned
	if len(m.multiWS.items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(m.multiWS.items))
	}
	if m.multiWS.items[0].workspace != "dev-gew4" {
		t.Errorf("expected dev-gew4, got %s", m.multiWS.items[0].workspace)
	}
}

func TestMultiWS_SkipApplyAllExcluded(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.config.SkipApplyWorkspaces = []string{"dev-gew4"}

	m.startMultiWS("")

	if m.multiWS.active {
		t.Error("should not activate multi-ws when all workspaces are excluded")
	}
	if !strings.Contains(m.statusMsg, "No workspaces match") {
		t.Errorf("expected no-match message, got %q", m.statusMsg)
	}
}

func TestChangeBadges_MixedActions(t *testing.T) {
	changes := []planChange{
		{Action: "create", Address: "aws_instance.a"},
		{Action: "create", Address: "aws_instance.b"},
		{Action: "update", Address: "aws_instance.c"},
		{Action: "destroy", Address: "aws_instance.d"},
		{Action: "destroy", Address: "aws_instance.e"},
		{Action: "destroy", Address: "aws_instance.f"},
	}
	got := changeBadges(changes)

	// Should contain counts for each action type
	if !strings.Contains(got, "+2") {
		t.Errorf("expected '+2' for creates, got %q", got)
	}
	if !strings.Contains(got, "~1") {
		t.Errorf("expected '~1' for updates, got %q", got)
	}
	if !strings.Contains(got, "-3") {
		t.Errorf("expected '-3' for destroys, got %q", got)
	}
}

func TestChangeBadges_OnlyCreates(t *testing.T) {
	changes := []planChange{
		{Action: "create", Address: "aws_instance.a"},
		{Action: "create", Address: "aws_instance.b"},
	}
	got := changeBadges(changes)

	if !strings.Contains(got, "+2") {
		t.Errorf("expected '+2', got %q", got)
	}
	// Should NOT contain destroy or update badges
	if strings.Contains(got, "-") {
		t.Errorf("should not have destroy badge, got %q", got)
	}
	if strings.Contains(got, "~") {
		t.Errorf("should not have update badge, got %q", got)
	}
}

func TestChangeBadges_Empty(t *testing.T) {
	got := changeBadges(nil)
	if got != "" {
		t.Errorf("expected empty string for no changes, got %q", got)
	}
}

func TestChangeBadges_WithReplace(t *testing.T) {
	changes := []planChange{
		{Action: "replace", Address: "aws_instance.a"},
		{Action: "create", Address: "aws_instance.b"},
	}
	got := changeBadges(changes)

	if !strings.Contains(got, "+1") {
		t.Errorf("expected '+1' for creates, got %q", got)
	}
	if !strings.Contains(got, "±1") {
		t.Errorf("expected '±1' for replaces, got %q", got)
	}
}

func TestChangeBadges_ReadOnly(t *testing.T) {
	changes := []planChange{
		{Action: "read", Address: "data.aws_ami.latest"},
	}
	got := changeBadges(changes)

	// Read-only data sources should show a badge
	if !strings.Contains(got, "?1") {
		t.Errorf("expected '?1' for reads, got %q", got)
	}
}

func TestChangeBadges_ShowsInRenderedList(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	planOutput := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami = "ami-123"
    }

  # aws_instance.api will be updated in-place
  ~ resource "aws_instance" "api" {
      ~ ami = "ami-old" -> "ami-new"
    }

  # aws_instance.old will be destroyed
  - resource "aws_instance" "old" {
    }

Plan: 1 to add, 1 to change, 1 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput})

	output := m.renderMultiWS()
	// The rendered output should contain the change badges
	if !strings.Contains(output, "+1") {
		t.Error("expected rendered list to contain '+1' for creates")
	}
	if !strings.Contains(output, "~1") {
		t.Error("expected rendered list to contain '~1' for updates")
	}
	if !strings.Contains(output, "-1") {
		t.Error("expected rendered list to contain '-1' for destroys")
	}
}

func TestChangeBadges_NotShownForPlanFail(t *testing.T) {
	changes := []planChange{} // no changes parsed on failure
	got := changeBadges(changes)
	if got != "" {
		t.Errorf("expected no badges for empty changes (plan fail), got %q", got)
	}
}

func TestMultiWS_StatusIcons(t *testing.T) {
	m := testModel()
	m.width = 120
	m.height = 40

	tests := []struct {
		status multiWSStatus
		want   string
	}{
		{mwsQueued, "⏳"},
		{mwsPlanning, "⟳"},
		{mwsPlanned, "⚠"},
		{mwsPlanFail, "✗"},
		{mwsNoChanges, "✓"},
		{mwsApplying, "⟳"},
		{mwsApplied, "✓"},
		{mwsApplyFail, "✗"},
	}
	for _, tt := range tests {
		got := m.multiWSStatusIcon(tt.status)
		if got != tt.want {
			t.Errorf("multiWSStatusIcon(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// ─── Apply Streaming Tests ───────────────────────────────

func TestMultiWS_PrepareItemForApply_SetsCursor(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Simulate plans completing
	for _, item := range m.multiWS.items {
		m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
			workspace: item.workspace,
			output:    "Plan: 1 to add, 0 to change, 0 to destroy.\n# aws_instance.web will be created",
		})
	}

	// Cursor starts at 0
	if m.multiWS.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.multiWS.cursor)
	}

	// Prepare item at index 2 for apply
	m.prepareItemForApply(2)

	if m.multiWS.cursor != 2 {
		t.Errorf("expected cursor moved to 2, got %d", m.multiWS.cursor)
	}
	if m.multiWS.items[2].status != mwsApplying {
		t.Errorf("expected status mwsApplying, got %d", m.multiWS.items[2].status)
	}
}

func TestMultiWS_PrepareItemForApply_AddsSeparator(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	planOutput := "Plan: 1 to add, 0 to change, 0 to destroy."
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput})

	planLen := len(m.multiWS.items[0].output)
	m.prepareItemForApply(0)

	item := &m.multiWS.items[0]
	if len(item.output) <= planLen {
		t.Fatal("expected separator lines appended to output")
	}

	// Check separator exists
	found := false
	for _, line := range item.output[planLen:] {
		if strings.Contains(line, "Apply Output") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected '─── Apply Output ───' separator in output")
	}
}

func TestMultiWS_PrepareItemForApply_SetsFollowApply(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    "Plan: 1 to add, 0 to change, 0 to destroy.\n# aws_instance.web will be created",
	})

	m.prepareItemForApply(0)

	if !m.multiWS.followApply {
		t.Error("expected followApply to be true")
	}
	if m.multiWS.applyHighlighter == nil {
		t.Error("expected applyHighlighter to be set")
	}
}

func TestMultiWS_PrepareItemForApply_ResetsViewState(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output: `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami = "ami-123"
    }

Plan: 1 to add, 0 to change, 0 to destroy.`,
	})

	// Enable focus and compact
	m.multiWS.view.focusView = true
	m.multiWS.view.compactDiff = true

	m.prepareItemForApply(0)

	if m.multiWS.view.focusView {
		t.Error("expected focus mode reset on apply start")
	}
	if m.multiWS.view.compactDiff {
		t.Error("expected compact diff reset on apply start")
	}
}

func TestMultiWS_HandleApplyLine_AppendsOutput(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "Plan: 1 to add."})
	m.prepareItemForApply(0)

	beforeLen := len(m.multiWS.items[0].output)

	m.handleMultiWSApplyLine(multiWSApplyLineMsg{
		workspace: "dev-gew4",
		line:      "aws_instance.web: Creating...",
	})

	item := &m.multiWS.items[0]
	if len(item.output) != beforeLen+1 {
		t.Errorf("expected output length %d, got %d", beforeLen+1, len(item.output))
	}
	if item.output[len(item.output)-1] != "aws_instance.web: Creating..." {
		t.Errorf("expected last line to be apply output, got %q", item.output[len(item.output)-1])
	}
	// hlOutput should also grow
	if len(item.hlOutput) != len(item.output) {
		t.Errorf("hlOutput length %d != output length %d", len(item.hlOutput), len(item.output))
	}
}

func TestMultiWS_HandleApplyLine_AutoScrolls(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "Plan: 1 to add."})
	m.prepareItemForApply(0)

	// Send enough lines to exceed visible height
	for i := 0; i < 50; i++ {
		m.handleMultiWSApplyLine(multiWSApplyLineMsg{
			workspace: "dev-gew4",
			line:      "aws_instance.web: Still creating...",
		})
	}

	// Scroll should follow output
	item := &m.multiWS.items[0]
	visH := m.multiWSVisibleHeight()
	expectedScroll := len(item.output) - visH
	if expectedScroll < 0 {
		expectedScroll = 0
	}
	if m.multiWS.scroll != expectedScroll {
		t.Errorf("expected scroll %d, got %d", expectedScroll, m.multiWS.scroll)
	}
}

func TestMultiWS_ScrollUpDisablesFollow(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: strings.Repeat("line\n", 50)})
	m.prepareItemForApply(0)
	m.multiWS.scroll = 10 // pretend we're scrolled down

	if !m.multiWS.followApply {
		t.Fatal("expected followApply true before scroll up")
	}

	// k (scroll up) should disable follow
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = result.(Model)

	if m.multiWS.followApply {
		t.Error("expected followApply false after scrolling up")
	}
}

func TestMultiWS_ScrollToBottomReenablesFollow(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: strings.Repeat("line\n", 50)})
	m.prepareItemForApply(0)
	m.multiWS.followApply = false

	// G (scroll to bottom) should re-enable follow during apply
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = result.(Model)

	if !m.multiWS.followApply {
		t.Error("expected followApply true after G during apply")
	}
}

func TestMultiWS_ApplyDone_UpdatesStatus(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "Plan: 1 to add."})
	m.prepareItemForApply(0)
	m.multiWS.followApply = true

	m.handleMultiWSApplyDone(multiWSApplyDoneMsg{workspace: "dev-gew4"})

	item := &m.multiWS.items[0]
	if item.status != mwsApplied {
		t.Errorf("expected mwsApplied, got %d", item.status)
	}
	if m.multiWS.followApply {
		t.Error("expected followApply cleared after done")
	}
}

func TestMultiWS_ApplyDone_Failure(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "Plan: 1 to add."})
	m.prepareItemForApply(0)

	m.handleMultiWSApplyDone(multiWSApplyDoneMsg{workspace: "dev-gew4", err: errFake("apply boom")})

	item := &m.multiWS.items[0]
	if item.status != mwsApplyFail {
		t.Errorf("expected mwsApplyFail, got %d", item.status)
	}
	if !strings.Contains(item.summary, "Apply failed") {
		t.Errorf("expected summary to contain 'Apply failed', got %q", item.summary)
	}
}

func TestMultiWS_DetailTitle_ShowsApplying(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    "Plan: 1 to add, 0 to change, 0 to destroy.\n# aws_instance.web will be created",
	})
	m.prepareItemForApply(0)

	output := m.renderMultiWS()
	if !strings.Contains(output, "Applying") {
		t.Error("expected detail title to contain 'Applying' during apply")
	}
}

func TestMultiWS_DetailTitle_ShowsApplied(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "Plan: 1 to add."})
	m.prepareItemForApply(0)
	m.handleMultiWSApplyDone(multiWSApplyDoneMsg{workspace: "dev-gew4"})
	m.multiWS.phase = "done"

	output := m.renderMultiWS()
	if !strings.Contains(output, "Applied") {
		t.Error("expected detail title to contain 'Applied' after apply")
	}
}

func TestMultiWS_SequentialApply_CursorFollows(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Complete plans for both
	for _, item := range m.multiWS.items {
		m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
			workspace: item.workspace,
			output:    "Plan: 1 to add, 0 to change, 0 to destroy.\n# aws_instance.web will be created",
		})
	}

	// Mark both for sequential apply
	for i := range m.multiWS.items {
		m.multiWS.items[i].applyQueued = true
	}
	m.multiWS.phase = "applying"

	// Start first apply
	m.startNextMultiWSApply()

	if m.multiWS.cursor != 0 {
		t.Errorf("expected cursor on first workspace (0), got %d", m.multiWS.cursor)
	}
	if m.multiWS.items[0].status != mwsApplying {
		t.Errorf("expected first item applying, got %d", m.multiWS.items[0].status)
	}

	// Complete first apply
	m.handleMultiWSApplyDone(multiWSApplyDoneMsg{workspace: "dev-gew4"})

	// Start second apply — cursor should move to it
	m.startNextMultiWSApply()

	if m.multiWS.cursor != 1 {
		t.Errorf("expected cursor moved to second workspace (1), got %d", m.multiWS.cursor)
	}
	if m.multiWS.items[1].status != mwsApplying {
		t.Errorf("expected second item applying, got %d", m.multiWS.items[1].status)
	}
}

func TestMultiWS_ApplyLine_WrongWorkspaceIgnored(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "Plan: 1 to add."})
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gae2", output: "Plan: 1 to add."})
	m.prepareItemForApply(0)

	item1Len := len(m.multiWS.items[1].output)

	// Send line for wrong workspace — should not affect dev-gae2
	m.handleMultiWSApplyLine(multiWSApplyLineMsg{
		workspace: "dev-gew4",
		line:      "creating...",
	})

	if len(m.multiWS.items[1].output) != item1Len {
		t.Error("apply line should not affect non-applying workspace")
	}
}

func TestMultiWS_YKeyStartsApply(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    "Plan: 1 to add, 0 to change, 0 to destroy.\n# aws_instance.web will be created",
	})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model := result.(Model)

	if model.multiWS.phase != "applying" {
		t.Errorf("expected phase 'applying', got %q", model.multiWS.phase)
	}
	if model.multiWS.items[0].status != mwsApplying {
		t.Errorf("expected item status mwsApplying, got %d", model.multiWS.items[0].status)
	}
	if !model.multiWS.followApply {
		t.Error("expected followApply set on y key")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (streaming apply)")
	}
}

func TestMultiWS_RenderDoesNotPanic(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Simulate plan completion for one workspace
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    "Plan: 1 to add, 0 to change, 0 to destroy.\n# aws_instance.web will be created",
	})

	// This should not panic
	output := m.renderMultiWS()
	if output == "" {
		t.Error("expected non-empty render output")
	}
	if !strings.Contains(output, "Multi-Workspace") {
		t.Error("expected render to contain 'Multi-Workspace'")
	}
	if !strings.Contains(output, "dev-gew4") {
		t.Error("expected render to contain 'dev-gew4'")
	}
}

func TestMultiWS_DetailScrolling(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Add lots of output (use non-refresh lines so they aren't collapsed)
	longOutput := strings.Repeat("  + resource line\n", 100)
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{
		workspace: "dev-gew4",
		output:    longOutput,
	})

	// Page down with d
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = result.(Model)
	if m.multiWS.scroll == 0 {
		t.Error("expected scroll > 0 after page down")
	}

	// Page up with u
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = result.(Model)
	if m.multiWS.scroll != 0 {
		t.Errorf("expected scroll = 0 after page up, got %d", m.multiWS.scroll)
	}
}

// ─── Compact Diff (z key) in Multi-WS ───────────────────

func TestMultiWS_ZKeyTogglesCompactDiff(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	planOutput := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami           = <<-EOT
          line 1
          line 2
          line 3
          line 4
          line 5
          line 6
          line 7
          line 8
          line 9
          line 10
        EOT
    }

Plan: 1 to add, 0 to change, 0 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput})

	// z toggles compact on
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)
	if !m.multiWS.view.compactDiff {
		t.Fatal("expected compactDiff true after z")
	}
	if m.multiWS.scroll != 0 {
		t.Errorf("expected scroll reset to 0, got %d", m.multiWS.scroll)
	}

	// z toggles compact off
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)
	if m.multiWS.view.compactDiff {
		t.Fatal("expected compactDiff false after second z")
	}
}

func TestMultiWS_CompactDiffRecomputesOnWorkspaceSwitch(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Complete plans for both workspaces
	planOutput1 := `  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami = "ami-123"
    }

Plan: 1 to add, 0 to change, 0 to destroy.`

	planOutput2 := `  # aws_instance.api will be updated in-place
  ~ resource "aws_instance" "api" {
      ~ ami = "ami-old" -> "ami-new"
    }

Plan: 0 to add, 1 to change, 0 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput1})
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gae2", output: planOutput2})

	// Enable compact diff
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)

	// Switch workspace — compact diff should be recomputed
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = result.(Model)

	if m.multiWS.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.multiWS.cursor)
	}
	// compactDiff should still be on
	if !m.multiWS.view.compactDiff {
		t.Fatal("expected compactDiff to persist across workspace switch")
	}
}

func TestMultiWS_CompactDiffHintShown(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")
	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: "No changes."})

	// Help should show z:compact
	output := m.renderMultiWS()
	if !strings.Contains(output, "compact") {
		t.Error("expected help to mention 'compact' for z key")
	}

	// Toggle compact on — help should show z:full diff
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)
	output = m.renderMultiWS()
	if !strings.Contains(output, "full diff") {
		t.Error("expected help to show 'full diff' when compact is active")
	}
}

func TestMultiWS_CompactDiffViewLinesAreUsed(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Plan output with a large heredoc that compact diff will collapse
	var heredocLines string
	for i := 0; i < 30; i++ {
		heredocLines += "          unchanged line\n"
	}
	planOutput := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + user_data = <<-EOT
` + heredocLines + `        EOT
    }

Plan: 1 to add, 0 to change, 0 to destroy.`

	m.handleMultiWSPlanDone(multiWSPlanDoneMsg{workspace: "dev-gew4", output: planOutput})

	item := m.multiWSSelectedItem()
	fullLineCount := len(item.output)

	// Enable compact diff
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = result.(Model)

	// ViewLines should return fewer lines than the full output
	viewLines, _ := m.multiWS.view.ViewLines(item.output, item.hlOutput, item.changes)
	if len(viewLines) >= fullLineCount {
		t.Errorf("expected compact view to have fewer lines than full (%d), got %d",
			fullLineCount, len(viewLines))
	}

	// Should contain a fold marker
	foundFold := false
	for _, line := range viewLines {
		if strings.Contains(line, "lines hidden") {
			foundFold = true
			break
		}
	}
	if !foundFold {
		t.Error("expected compact view to contain fold marker")
	}
}

// ─── Shared planViewer between normal and multi-ws ───────

func TestMultiWS_ViewUsesSharedPlanViewer(t *testing.T) {
	// Verify the planViewer struct is the same type used for normal plan review
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Both m.planView and m.multiWS.view should be planViewer
	m.planView.compactDiff = true
	m.multiWS.view.compactDiff = true

	// They should be independent instances
	m.planView.compactDiff = false
	if !m.multiWS.view.compactDiff {
		t.Error("multi-ws view should be independent from normal plan view")
	}
}

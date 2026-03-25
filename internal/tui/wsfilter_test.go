package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/terraform"
)

// baseFilterModel returns a model with several workspaces for filter testing.
func baseFilterModel() Model {
	m := Model{
		width:     120,
		height:    40,
		workspace: "default",
		workspaces: &terraform.WorkspaceInfo{
			Current:    "default",
			Workspaces: []string{"default", "dev-us", "dev-eu", "staging", "prod-us", "prod-eu"},
		},
		activePanel: PanelWorkspaces,
		panels:      makePanels(),
	}
	m.rebuildWorkspacesPanel()
	return m
}

// --- Opening filter ---

func TestWsFilter_SlashOpensInput(t *testing.T) {
	m := baseFilterModel()
	got := sendKey(m, "/")

	if !got.showInput {
		t.Fatal("expected input overlay to be shown")
	}
	if got.inputAction != "workspace_filter" {
		t.Errorf("inputAction = %q, want %q", got.inputAction, "workspace_filter")
	}
	if got.inputPrompt != "Filter Workspaces" {
		t.Errorf("inputPrompt = %q, want %q", got.inputPrompt, "Filter Workspaces")
	}
}

func TestWsFilter_SlashPreFillsExistingFilter(t *testing.T) {
	m := baseFilterModel()
	m.wsFilter = "dev"
	got := sendKey(m, "/")

	if !got.showInput {
		t.Fatal("expected input overlay to be shown")
	}
	if got.inputValue != "dev" {
		t.Errorf("inputValue = %q, want %q (should pre-fill current filter)", got.inputValue, "dev")
	}
}

// --- Applying filter ---

func TestWsFilter_SubmitFiltersWorkspaces(t *testing.T) {
	m := baseFilterModel()
	m.workspace = "dev-us" // active workspace matches filter

	// Open filter, type "dev", submit
	m.showInput = true
	m.inputAction = "workspace_filter"
	m.inputValue = "dev"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)

	if got.showInput {
		t.Fatal("input overlay should be dismissed")
	}
	if got.wsFilter != "dev" {
		t.Errorf("wsFilter = %q, want %q", got.wsFilter, "dev")
	}

	// Panel should only show workspaces containing "dev"
	panel := got.panels[PanelWorkspaces]
	if len(panel.Items) != 2 {
		t.Fatalf("expected 2 filtered workspaces, got %d", len(panel.Items))
	}
	if panel.Items[0].Label != "dev-us" {
		t.Errorf("first item = %q, want %q", panel.Items[0].Label, "dev-us")
	}
	if panel.Items[1].Label != "dev-eu" {
		t.Errorf("second item = %q, want %q", panel.Items[1].Label, "dev-eu")
	}
}

func TestWsFilter_SubmitProdFilter(t *testing.T) {
	m := baseFilterModel()
	m.workspace = "prod-us" // active workspace matches filter

	m.showInput = true
	m.inputAction = "workspace_filter"
	m.inputValue = "prod"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)

	panel := got.panels[PanelWorkspaces]
	if len(panel.Items) != 2 {
		t.Fatalf("expected 2 filtered workspaces, got %d", len(panel.Items))
	}
	for _, item := range panel.Items {
		if !strings.Contains(item.Label, "prod") {
			t.Errorf("unexpected workspace in filtered list: %q", item.Label)
		}
	}
}

func TestWsFilter_CaseInsensitive(t *testing.T) {
	m := baseFilterModel()
	m.workspace = "dev-us" // active workspace matches filter

	m.showInput = true
	m.inputAction = "workspace_filter"
	m.inputValue = "DEV"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)

	panel := got.panels[PanelWorkspaces]
	if len(panel.Items) != 2 {
		t.Fatalf("expected 2 filtered workspaces (case-insensitive), got %d", len(panel.Items))
	}
}

// --- Clearing filter ---

func TestWsFilter_EmptySubmitClearsFilter(t *testing.T) {
	m := baseFilterModel()
	m.wsFilter = "dev"
	m.rebuildWorkspacesPanel() // should show only 2

	// Open filter, clear it, submit empty
	m.showInput = true
	m.inputAction = "workspace_filter"
	m.inputValue = ""
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)

	if got.wsFilter != "" {
		t.Errorf("wsFilter should be cleared, got %q", got.wsFilter)
	}

	panel := got.panels[PanelWorkspaces]
	if len(panel.Items) != 6 {
		t.Fatalf("expected all 6 workspaces after clearing filter, got %d", len(panel.Items))
	}
}

// --- Filter survives data reload ---

func TestWsFilter_PersistsAfterDataReload(t *testing.T) {
	m := baseFilterModel()
	m.workspace = "dev-us" // active workspace matches filter
	m.wsFilter = "dev"
	m.rebuildWorkspacesPanel()

	// Simulate data reload — workspaces data comes in again
	m.workspaces = &terraform.WorkspaceInfo{
		Current:    "dev-us",
		Workspaces: []string{"default", "dev-us", "dev-eu", "staging", "prod-us", "prod-eu"},
	}
	m.rebuildWorkspacesPanel()

	panel := m.panels[PanelWorkspaces]
	if len(panel.Items) != 2 {
		t.Fatalf("filter should persist after rebuild, expected 2, got %d", len(panel.Items))
	}
}

// --- Active workspace always shown ---

func TestWsFilter_ActiveWorkspaceAlwaysVisible(t *testing.T) {
	m := baseFilterModel()
	m.workspace = "staging" // active ws doesn't match filter
	m.wsFilter = "dev"
	m.rebuildWorkspacesPanel()

	panel := m.panels[PanelWorkspaces]
	// Should show: staging (active, even though no match) + dev-us + dev-eu
	found := false
	for _, item := range panel.Items {
		if item.Label == "staging" {
			found = true
		}
	}
	if !found {
		t.Error("active workspace 'staging' should always be visible even when filtered out")
	}
	if len(panel.Items) != 3 {
		t.Fatalf("expected 3 workspaces (active + 2 matching), got %d", len(panel.Items))
	}
}

// --- Hint bar shows filter ---

func TestWsFilter_ContextKeyHintShowsFilter(t *testing.T) {
	m := baseFilterModel()
	hints := contextKeysFor(PanelWorkspaces, &m)

	// Should have / key
	found := false
	for _, h := range hints {
		if h.Key == "/" {
			found = true
		}
	}
	if !found {
		t.Error("workspace context keys should include /")
	}
}

func TestWsFilter_ContextKeyHintShowsClear(t *testing.T) {
	m := baseFilterModel()
	m.wsFilter = "dev"
	hints := contextKeysFor(PanelWorkspaces, &m)

	// When filter is active, should show current filter in hint
	found := false
	for _, h := range hints {
		if h.Key == "/" && strings.Contains(h.Desc, "dev") {
			found = true
		}
	}
	if !found {
		t.Error("workspace context keys should show current filter value when active")
	}
}

// --- Workspace switch clears filter ---

func TestWsFilter_SwitchWorkspaceClearsFilter(t *testing.T) {
	m := baseFilterModel()
	m.wsFilter = "dev"
	m.rebuildWorkspacesPanel()

	// Simulate successful workspace switch via cmdDoneMsg
	msg := cmdDoneMsg{title: "Workspace: dev-us", err: nil, output: "Switched to workspace \"dev-us\"."}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.wsFilter != "" {
		t.Errorf("filter should be cleared after workspace switch, got %q", updated.wsFilter)
	}
}

// --- Busy guard: filter should work even while busy ---

func TestWsFilter_SlashAllowedWhileBusy(t *testing.T) {
	m := baseFilterModel()
	m.isLoading = true
	got := sendKey(m, "/")

	if !got.showInput {
		t.Fatal("/ should be allowed even while busy (it's not a terraform operation)")
	}
}

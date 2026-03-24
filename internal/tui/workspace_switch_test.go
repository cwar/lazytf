package tui

import (
	"testing"

	"github.com/cwar/lazytf/internal/terraform"
)

// TestWorkspaceSwitchUpdatesVarFile verifies that after a workspace switch
// completes (cmdDoneMsg), the selectedVarFile is eagerly updated to match
// the new workspace — even before loadAllData/dataLoadedMsg finishes.
//
// This prevents a race where pressing 'a' (apply) right after switching
// workspaces would use the OLD workspace's var file.
func TestWorkspaceSwitchUpdatesVarFile(t *testing.T) {
	m := Model{
		workspace:       "dev",
		selectedVarFile: "/proj/dev.tfvars",
		varFileManual:   false,
		width:           120,
		height:          40,
		files: []terraform.TfFile{
			{Name: "dev.tfvars", Path: "/proj/dev.tfvars", IsVars: true, Dir: ""},
			{Name: "staging.tfvars", Path: "/proj/staging.tfvars", IsVars: true, Dir: ""},
		},
		panels: makePanels(),
	}

	// Simulate cmdDoneMsg from a successful workspace switch
	msg := cmdDoneMsg{title: "Workspace: staging", err: nil, output: "Switched to workspace \"staging\"."}
	result, _ := m.Update(msg)
	updated := result.(Model)

	// After workspace switch completes, workspace and var file should be
	// updated EAGERLY — not deferred until dataLoadedMsg.
	if updated.workspace != "staging" {
		t.Errorf("workspace not updated: got %q, want %q", updated.workspace, "staging")
	}
	if updated.selectedVarFile != "/proj/staging.tfvars" {
		t.Errorf("selectedVarFile not updated: got %q, want %q", updated.selectedVarFile, "/proj/staging.tfvars")
	}
}

// TestWorkspaceSwitchFailureKeepsOldState verifies that if a workspace switch
// fails, we don't update the workspace or var file.
func TestWorkspaceSwitchFailureKeepsOldState(t *testing.T) {
	m := Model{
		workspace:       "dev",
		selectedVarFile: "/proj/dev.tfvars",
		varFileManual:   false,
		width:           120,
		height:          40,
		files: []terraform.TfFile{
			{Name: "dev.tfvars", Path: "/proj/dev.tfvars", IsVars: true, Dir: ""},
			{Name: "staging.tfvars", Path: "/proj/staging.tfvars", IsVars: true, Dir: ""},
		},
		panels: makePanels(),
	}

	// Simulate cmdDoneMsg from a FAILED workspace switch
	msg := cmdDoneMsg{
		title:  "Workspace: staging",
		err:    errFake("workspace switch failed"),
		output: "Error: ...",
	}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.workspace != "dev" {
		t.Errorf("workspace should stay unchanged on error: got %q, want %q", updated.workspace, "dev")
	}
	if updated.selectedVarFile != "/proj/dev.tfvars" {
		t.Errorf("selectedVarFile should stay unchanged on error: got %q, want %q", updated.selectedVarFile, "/proj/dev.tfvars")
	}
}

// TestWorkspaceSwitchPreservesManualVarFile verifies that if the user manually
// selected a var file, a workspace switch doesn't auto-override it.
// Note: handlePanelAction already resets varFileManual=false before the switch,
// so by the time cmdDoneMsg arrives, auto-select SHOULD run. This test
// verifies the full behavior: manual mode was already cleared.
func TestWorkspaceSwitchPreservesManualVarFile(t *testing.T) {
	m := Model{
		workspace:       "dev",
		selectedVarFile: "/proj/custom.tfvars",
		varFileManual:   true, // user explicitly chose this
		width:           120,
		height:          40,
		files: []terraform.TfFile{
			{Name: "dev.tfvars", Path: "/proj/dev.tfvars", IsVars: true, Dir: ""},
			{Name: "staging.tfvars", Path: "/proj/staging.tfvars", IsVars: true, Dir: ""},
			{Name: "custom.tfvars", Path: "/proj/custom.tfvars", IsVars: true, Dir: ""},
		},
		panels: makePanels(),
	}

	// If manual mode is still set (e.g. someone calls workspace select
	// from a different path), auto-select should NOT override.
	msg := cmdDoneMsg{title: "Workspace: staging", err: nil, output: "Switched to workspace \"staging\"."}
	result, _ := m.Update(msg)
	updated := result.(Model)

	// Workspace should still be updated
	if updated.workspace != "staging" {
		t.Errorf("workspace not updated: got %q, want %q", updated.workspace, "staging")
	}
	// But var file should NOT change since varFileManual is true
	if updated.selectedVarFile != "/proj/custom.tfvars" {
		t.Errorf("manual var file was overridden: got %q, want %q", updated.selectedVarFile, "/proj/custom.tfvars")
	}
}

// makePanels creates minimal panels for testing.
func makePanels() []*SubPanel {
	panels := make([]*SubPanel, PanelCount)
	for i := PanelID(0); i < PanelCount; i++ {
		panels[i] = &SubPanel{ID: i}
	}
	return panels
}

type errFake string

func (e errFake) Error() string { return string(e) }

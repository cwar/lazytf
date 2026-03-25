package tui

import (
	"strings"
	"testing"

	"github.com/cwar/lazytf/internal/config"
	"github.com/cwar/lazytf/internal/terraform"
)

// ─── Skipped Workspaces: Sorted to End ───────────────────

func TestWorkspaces_SkippedSortedToEnd(t *testing.T) {
	m := Model{
		width:     120,
		height:    40,
		workspace: "default",
		workspaces: &terraform.WorkspaceInfo{
			Current:    "default",
			Workspaces: []string{"default", "alpha", "beta", "gamma", "delta"},
		},
		config: config.Config{
			SkipApplyWorkspaces: []string{"alpha", "gamma"},
		},
		panels: makePanels(),
	}
	m.rebuildWorkspacesPanel()

	panel := m.panels[PanelWorkspaces]
	if len(panel.Items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(panel.Items))
	}

	// Non-skipped workspaces should come first: default, beta, delta
	// Skipped workspaces should come last: alpha, gamma
	var labels []string
	for _, item := range panel.Items {
		// Data is the clean workspace name
		labels = append(labels, item.Data.(string))
	}

	// First 3 should be the non-skipped ones (in original order)
	nonSkipped := labels[:3]
	skipped := labels[3:]

	for _, ws := range nonSkipped {
		if m.config.IsSkipApply(ws) {
			t.Errorf("non-skipped section contains skipped workspace %q; order: %v", ws, labels)
		}
	}
	for _, ws := range skipped {
		if !m.config.IsSkipApply(ws) {
			t.Errorf("skipped section contains non-skipped workspace %q; order: %v", ws, labels)
		}
	}
}

func TestWorkspaces_SkippedPreserveRelativeOrder(t *testing.T) {
	m := Model{
		width:     120,
		height:    40,
		workspace: "default",
		workspaces: &terraform.WorkspaceInfo{
			Current:    "default",
			Workspaces: []string{"default", "alpha", "beta", "gamma", "delta"},
		},
		config: config.Config{
			SkipApplyWorkspaces: []string{"gamma", "alpha"},
		},
		panels: makePanels(),
	}
	m.rebuildWorkspacesPanel()

	panel := m.panels[PanelWorkspaces]
	var labels []string
	for _, item := range panel.Items {
		labels = append(labels, item.Data.(string))
	}

	// Non-skipped: default, beta, delta (original order preserved)
	if labels[0] != "default" || labels[1] != "beta" || labels[2] != "delta" {
		t.Errorf("non-skipped order wrong: got %v", labels[:3])
	}
	// Skipped: alpha, gamma (original order preserved)
	if labels[3] != "alpha" || labels[4] != "gamma" {
		t.Errorf("skipped order wrong: got %v", labels[3:])
	}
}

// ─── Skipped Workspaces: Dim Flag ────────────────────────

func TestWorkspaces_SkippedHaveDimFlag(t *testing.T) {
	m := Model{
		width:     120,
		height:    40,
		workspace: "default",
		workspaces: &terraform.WorkspaceInfo{
			Current:    "default",
			Workspaces: []string{"default", "dev", "staging", "prod"},
		},
		config: config.Config{
			SkipApplyWorkspaces: []string{"staging"},
		},
		panels: makePanels(),
	}
	m.rebuildWorkspacesPanel()

	panel := m.panels[PanelWorkspaces]
	for _, item := range panel.Items {
		ws := item.Data.(string)
		if ws == "staging" && !item.Dim {
			t.Error("expected skipped workspace 'staging' to have Dim=true")
		}
		if ws != "staging" && item.Dim {
			t.Errorf("expected non-skipped workspace %q to have Dim=false", ws)
		}
	}
}

// ─── Skipped Workspaces: Faded Rendering ─────────────────

func TestWorkspaces_SkippedRenderedFaded(t *testing.T) {
	m := Model{
		width:     120,
		height:    40,
		workspace: "default",
		workspaces: &terraform.WorkspaceInfo{
			Current:    "default",
			Workspaces: []string{"default", "prod"},
		},
		config: config.Config{
			SkipApplyWorkspaces: []string{"prod"},
		},
		activePanel: PanelWorkspaces,
		focus:       FocusLeft,
		panels:      makePanels(),
	}
	m.rebuildWorkspacesPanel()

	// Render workspace panel — prod should appear faded (using DimItem style)
	panel := m.panels[PanelWorkspaces]
	panel.Height = 10
	output := panel.Render(40, true, true)

	// "prod" should be present but the "(skip)" part should use DimItem styling.
	// Non-selected dim items should NOT use the normal item white foreground.
	if !strings.Contains(output, "prod") {
		t.Error("expected 'prod' to appear in rendered output")
	}
	if !strings.Contains(output, "skip") {
		t.Error("expected 'skip' indicator for prod in rendered output")
	}
}

// ─── Filter + Skip Interaction ───────────────────────────

func TestWorkspaces_SkippedSortedToEndWithFilter(t *testing.T) {
	m := Model{
		width:     120,
		height:    40,
		workspace: "default",
		workspaces: &terraform.WorkspaceInfo{
			Current:    "default",
			Workspaces: []string{"default", "dev-us", "dev-eu", "staging", "prod-us", "prod-eu"},
		},
		config: config.Config{
			SkipApplyWorkspaces: []string{"dev-eu", "prod-eu"},
		},
		wsFilter: "dev",
		panels:   makePanels(),
	}
	m.rebuildWorkspacesPanel()

	panel := m.panels[PanelWorkspaces]
	var labels []string
	for _, item := range panel.Items {
		labels = append(labels, item.Data.(string))
	}

	// With filter "dev": dev-us (non-skipped) should come before dev-eu (skipped)
	// "default" is the active workspace and doesn't match the filter, but is always shown
	// It IS non-skipped, so it should appear before the skipped ones.
	foundSkipBoundary := false
	for i, ws := range labels {
		isSkipped := m.config.IsSkipApply(ws)
		if isSkipped {
			foundSkipBoundary = true
		}
		if foundSkipBoundary && !isSkipped {
			t.Errorf("non-skipped workspace %q appears after skipped workspace at index %d; order: %v", ws, i, labels)
		}
	}
}

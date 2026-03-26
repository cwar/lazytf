package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPanelRender_TruncatesLongLabels(t *testing.T) {
	width := 40
	p := &SubPanel{
		ID: PanelResources,
		Items: []PanelItem{
			{Label: "short"},
			{Label: "gke (terraform-google-modules/kubernetes-engine/google//modules/private-cluster-update-variant)"},
			{Icon: "📦", Label: "another-very-long-module-name-that-should-be-truncated-to-fit"},
		},
		Cursor: 0,
		Scroll: 0,
		Height: 7, // title + 3 items + 2 pad + counter
	}

	rendered := p.Render(width, true, true)
	lines := strings.Split(rendered, "\n")

	// Every line in the rendered panel must fit within `width` visible columns
	for i, line := range lines {
		visWidth := lipgloss.Width(line)
		if visWidth > width {
			t.Errorf("line %d exceeds width %d (got %d): %q", i, width, visWidth, line)
		}
	}
}

func TestPanelRender_ShortLabelsUnchanged(t *testing.T) {
	width := 40
	label := "short-resource"
	p := &SubPanel{
		ID: PanelResources,
		Items: []PanelItem{
			{Label: label},
		},
		Cursor: 0,
		Scroll: 0,
		Height: 5,
	}

	rendered := p.Render(width, false, false)

	// The short label should appear in the output untruncated
	if !strings.Contains(rendered, label) {
		t.Errorf("expected label %q in rendered output", label)
	}
}

func TestPanelRender_TruncatedLineShowsEllipsis(t *testing.T) {
	width := 30
	p := &SubPanel{
		ID: PanelFiles,
		Items: []PanelItem{
			{Label: "this-is-a-very-long-filename-that-exceeds-panel-width.tf"},
		},
		Cursor: 0,
		Scroll: 0,
		Height: 5,
	}

	rendered := p.Render(width, true, true)

	// The long label should be truncated with an ellipsis
	if !strings.Contains(rendered, "…") {
		t.Errorf("expected ellipsis in truncated output, got:\n%s", rendered)
	}
}

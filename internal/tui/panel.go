package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/cwar/lazytf/internal/ui"
)

// PanelID identifies a sub-panel in the left column.
type PanelID int

const (
	PanelStatus PanelID = iota
	PanelFiles
	PanelResources // merged Resources + Modules (modules as group headers)
	PanelWorkspaces
	PanelVarFiles
	PanelHistory // command history (recent plans, applies, etc.)
	PanelCount   // sentinel — number of panels
)

// panelMeta defines static panel metadata.
type panelMeta struct {
	Icon     string
	Label    string
	MinRows  int // minimum visible item rows
	FlexGrow int // relative weight for extra space
}

var panelDefs = map[PanelID]panelMeta{
	PanelStatus:     {"✓", "Status", 1, 0},
	PanelFiles:      {"📄", "Files", 3, 3},
	PanelResources:  {"🏗", "Resources", 3, 4},
	PanelWorkspaces: {"📁", "Workspaces", 2, 1},
	PanelVarFiles:   {"⚙", "Var Files", 2, 1},
	PanelHistory:    {"📋", "History", 2, 2},
}

// SubPanel holds the state of one stacked panel.
type SubPanel struct {
	ID     PanelID
	Items  []PanelItem
	Cursor int
	Scroll int
	Height int // allocated display height (set during layout)
}

// PanelItem is a single selectable item in a sub-panel.
type PanelItem struct {
	Label string // display text
	Data  any    // associated object
	Icon  string // optional prefix icon
	Dim   bool   // render with faded/gray styling (e.g. skipped workspaces)
}

// VisibleHeight returns the number of item rows visible (height minus border/title).
func (p *SubPanel) VisibleHeight() int {
	h := p.Height - 2 // title line + bottom counter
	if h < 1 {
		return 1
	}
	return h
}

// EnsureCursorVisible adjusts scroll so cursor is visible.
func (p *SubPanel) EnsureCursorVisible() {
	vis := p.VisibleHeight()
	if p.Cursor < p.Scroll {
		p.Scroll = p.Cursor
	}
	if p.Cursor >= p.Scroll+vis {
		p.Scroll = p.Cursor - vis + 1
	}
}

// MoveDown moves cursor down.
func (p *SubPanel) MoveDown() {
	if p.Cursor < len(p.Items)-1 {
		p.Cursor++
		p.EnsureCursorVisible()
	}
}

// MoveUp moves cursor up.
func (p *SubPanel) MoveUp() {
	if p.Cursor > 0 {
		p.Cursor--
		p.EnsureCursorVisible()
	}
}

// SelectedItem returns the currently selected item or nil.
func (p *SubPanel) SelectedItem() *PanelItem {
	if p.Cursor >= 0 && p.Cursor < len(p.Items) {
		return &p.Items[p.Cursor]
	}
	return nil
}

// Render renders the sub-panel at the given width.
// isActive indicates this is the focused panel on the left side.
// isLeftFocused indicates the left column is focused (vs right detail pane).
func (p *SubPanel) Render(width int, isActive, isLeftFocused bool) string {
	meta := panelDefs[p.ID]
	vis := p.VisibleHeight()

	// --- Title line ---
	numKey := fmt.Sprintf("[%d]", int(p.ID)+1)
	var titleLine string

	if isActive && isLeftFocused {
		titleLine = ui.StatusKey.Render(numKey) +
			ui.PanelTitle.Render(meta.Icon+" "+meta.Label)
	} else if isActive {
		titleLine = ui.DimItem.Render(numKey) +
			ui.InactivePanelTitle.Render(meta.Icon+" "+meta.Label)
	} else {
		titleLine = ui.DimItem.Render(numKey+" "+meta.Icon+" "+meta.Label)
	}

	// Fill title to width
	titleWidth := lipgloss.Width(titleLine)
	if titleWidth < width {
		titleLine += strings.Repeat("─", width-titleWidth)
	}

	// --- Item lines ---
	var lines []string
	end := p.Scroll + vis
	if end > len(p.Items) {
		end = len(p.Items)
	}

	for i := p.Scroll; i < end; i++ {
		item := p.Items[i]
		line := " "
		if item.Icon != "" {
			line += item.Icon + " "
		}
		line += item.Label

		// Truncate or pad to width
		lineWidth := lipgloss.Width(line)
		if lineWidth > width {
			line = ansi.Truncate(line, width-1, "…")
			// Re-pad after truncation (Truncate may leave it slightly short)
			lineWidth = lipgloss.Width(line)
			if lineWidth < width {
				line += strings.Repeat(" ", width-lineWidth)
			}
		} else if lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}

		if i == p.Cursor && isActive && isLeftFocused {
			line = ui.SelectedItem.Render(line)
		} else if i == p.Cursor && isActive {
			// Selected but left not focused — dim highlight
			line = lipgloss.NewStyle().
				Foreground(ui.White).
				Background(lipgloss.Color("#1A2744")).
				Render(line)
		} else if item.Dim {
			line = ui.DimItem.Render(line)
		}

		lines = append(lines, line)
	}

	// Pad empty rows
	for len(lines) < vis {
		lines = append(lines, strings.Repeat(" ", width))
	}

	// --- Counter line ---
	counter := ""
	if len(p.Items) > 0 {
		counter = fmt.Sprintf("%d of %d", p.Cursor+1, len(p.Items))
	} else {
		counter = "0 of 0"
	}
	counterPad := width - len(counter)
	if counterPad < 0 {
		counterPad = 0
	}
	counterLine := ui.DimItem.Render(strings.Repeat("─", counterPad) + counter)

	// Assemble
	all := []string{titleLine}
	all = append(all, lines...)
	all = append(all, counterLine)

	return strings.Join(all, "\n")
}

// AllocatePanelHeights distributes available height among panels.
// Each panel gets at least its MinRows + 2 (title + counter).
// Extra space goes to panels with FlexGrow > 0, proportionally.
func AllocatePanelHeights(panels []*SubPanel, totalHeight int) {
	// Phase 1: give each panel its minimum
	used := 0
	for _, p := range panels {
		meta := panelDefs[p.ID]
		minH := meta.MinRows + 2
		// If panel has items, try to show them
		needH := len(p.Items) + 2
		if needH < minH {
			needH = minH
		}
		p.Height = minH
		used += minH
	}

	// Phase 2: distribute remaining space by flex grow
	remaining := totalHeight - used
	if remaining <= 0 {
		return
	}

	totalFlex := 0
	for _, p := range panels {
		meta := panelDefs[p.ID]
		totalFlex += meta.FlexGrow
	}
	if totalFlex == 0 {
		return
	}

	for _, p := range panels {
		meta := panelDefs[p.ID]
		if meta.FlexGrow == 0 {
			continue
		}
		extra := remaining * meta.FlexGrow / totalFlex
		// But don't give more than needed
		needH := len(p.Items) + 2
		maxExtra := needH - p.Height
		if maxExtra < 0 {
			maxExtra = 0
		}
		if extra > maxExtra {
			extra = maxExtra
		}
		p.Height += extra
	}

	// Phase 3: any leftover goes to panels that still need it
	used = 0
	for _, p := range panels {
		used += p.Height
	}
	remaining = totalHeight - used
	if remaining <= 0 {
		return
	}
	for _, p := range panels {
		meta := panelDefs[p.ID]
		if meta.FlexGrow == 0 {
			continue
		}
		need := len(p.Items) + 2 - p.Height
		if need <= 0 {
			continue
		}
		give := need
		if give > remaining {
			give = remaining
		}
		p.Height += give
		remaining -= give
		if remaining <= 0 {
			break
		}
	}

	// Phase 4: if STILL remaining, distribute evenly to flex panels
	if remaining > 0 {
		for _, p := range panels {
			meta := panelDefs[p.ID]
			if meta.FlexGrow == 0 {
				continue
			}
			give := remaining / 2
			if give < 1 {
				give = 1
			}
			p.Height += give
			remaining -= give
			if remaining <= 0 {
				break
			}
		}
	}
}

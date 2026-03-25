package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cwar/lazytf/internal/ui"
)

// wsPicker holds state for the workspace selection screen shown before
// multi-workspace planning begins. Users can toggle individual workspaces
// on/off to fine-tune which workspaces will be planned.
type wsPicker struct {
	workspaces []string // workspace names available for selection
	varFiles   []string // matched var file for each workspace (may be "")
	checked    []bool   // selection state per workspace
	cursor     int      // cursor position in the list
	scroll     int      // scroll offset for long lists
}

// newWSPicker creates a picker with all workspaces selected by default.
func newWSPicker(workspaces, varFiles []string) wsPicker {
	checked := make([]bool, len(workspaces))
	for i := range checked {
		checked[i] = true
	}
	return wsPicker{
		workspaces: workspaces,
		varFiles:   varFiles,
		checked:    checked,
	}
}

// toggle flips the checked state of the workspace at idx.
func (p *wsPicker) toggle(idx int) {
	if idx >= 0 && idx < len(p.checked) {
		p.checked[idx] = !p.checked[idx]
	}
}

// toggleAll checks all workspaces if any are unchecked, otherwise unchecks all.
func (p *wsPicker) toggleAll() {
	allChecked := true
	for _, c := range p.checked {
		if !c {
			allChecked = false
			break
		}
	}
	for i := range p.checked {
		p.checked[i] = !allChecked
	}
}

// selectedCount returns how many workspaces are currently checked.
func (p *wsPicker) selectedCount() int {
	n := 0
	for _, c := range p.checked {
		if c {
			n++
		}
	}
	return n
}

// selectedWorkspacesAndVarFiles returns the workspaces and var files
// for only the checked items.
func (p *wsPicker) selectedWorkspacesAndVarFiles() ([]string, []string) {
	var ws, vf []string
	for i, c := range p.checked {
		if c {
			ws = append(ws, p.workspaces[i])
			vf = append(vf, p.varFiles[i])
		}
	}
	return ws, vf
}

// ─── Key Handling ────────────────────────────────────────

// handleWSPickerKey handles keys during the workspace selection phase.
func (m Model) handleWSPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	p := &m.multiWS.picker

	switch key {
	case "ctrl+c":
		m.closeMultiWS()
		return m, tea.Quit

	case "esc", "q":
		m.closeMultiWS()
		return m, nil

	case "j", "down":
		if p.cursor < len(p.workspaces)-1 {
			p.cursor++
		} else {
			p.cursor = 0 // wrap
		}
		return m, nil

	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		} else {
			p.cursor = len(p.workspaces) - 1 // wrap
		}
		return m, nil

	case " ", "tab":
		p.toggle(p.cursor)
		return m, nil

	case "a":
		p.toggleAll()
		return m, nil

	case "enter":
		return m.confirmMultiWSPicker()
	}

	return m, nil
}

// confirmMultiWSPicker transitions from the selection phase to planning.
// It builds multiWSItems from the checked workspaces and kicks off
// parallel plan commands.
func (m Model) confirmMultiWSPicker() (tea.Model, tea.Cmd) {
	selected, varFiles := m.multiWS.picker.selectedWorkspacesAndVarFiles()
	if len(selected) == 0 {
		m.statusMsg = ui.ErrorStyle.Render("No workspaces selected")
		return m, nil
	}

	// Build items with matched var files and plan file paths
	items := make([]multiWSItem, len(selected))
	for i, ws := range selected {
		items[i] = multiWSItem{
			workspace: ws,
			varFile:   varFiles[i],
			planFile:  tempPlanFile(),
			status:    mwsPlanning,
		}
	}

	// Create fresh context for planning
	ctx, cancel := newMultiWSContext()

	m.multiWS.items = items
	m.multiWS.phase = "planning"
	m.multiWS.cancel = cancel

	// Build batch of plan commands with semaphore
	sem := make(chan struct{}, multiWSConcurrency)
	var cmds []tea.Cmd
	for _, item := range items {
		ws := item.workspace
		vf := item.varFile
		pf := item.planFile
		cmds = append(cmds, func() tea.Msg {
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			out, err := m.runner.PlanSaveWithWorkspace(ctx, ws, vf, pf, false)
			return multiWSPlanDoneMsg{workspace: ws, output: out, err: err}
		})
	}

	return m, tea.Batch(cmds...)
}

// ─── Rendering ───────────────────────────────────────────

// renderWSPicker renders the full-screen workspace selection overlay.
func (m Model) renderWSPicker() string {
	p := &m.multiWS.picker
	sel := p.selectedCount()
	total := len(p.workspaces)

	// ── Title bar ──
	filterInfo := ""
	if m.multiWS.filter != "" {
		filterInfo = fmt.Sprintf(" (filter: %q)", m.multiWS.filter)
	}
	titleText := fmt.Sprintf(" ⚡ Select Workspaces%s ", filterInfo)
	countText := fmt.Sprintf(" %d/%d selected ", sel, total)
	titlePad := m.width - lipgloss.Width(titleText) - lipgloss.Width(countText)
	if titlePad < 0 {
		titlePad = 0
	}
	titleBar := ui.PanelTitle.Render(titleText) +
		ui.DimItem.Render(strings.Repeat("─", titlePad)) +
		ui.StatusKey.Render(countText)

	// ── Workspace list ──
	contentH := m.height - 3 // title + status + help
	var lines []string

	for i, ws := range p.workspaces {
		// Checkbox
		check := ui.SuccessStyle.Render("✓")
		if !p.checked[i] {
			check = ui.DimItem.Render("○")
		}

		// Cursor indicator
		cursor := "  "
		if i == p.cursor {
			cursor = ui.StatusKey.Render("▸ ")
		}

		// Var file
		varInfo := ""
		if p.varFiles[i] != "" {
			varInfo = ui.DimItem.Render("  " + filepath.Base(p.varFiles[i]))
		}

		line := fmt.Sprintf("%s %s %s%s", cursor, check, ws, varInfo)

		// Highlight selected row
		if i == p.cursor {
			// Pad to width and apply highlight
			lineWidth := lipgloss.Width(line)
			pad := m.width - lineWidth
			if pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			line = ui.SelectedItem.Render(line)
		}

		lines = append(lines, line)
	}

	// Pad to content height
	for len(lines) < contentH {
		lines = append(lines, "")
	}
	if len(lines) > contentH {
		lines = lines[:contentH]
	}

	content := strings.Join(lines, "\n")

	// ── Status bar ──
	statusText := ""
	if m.statusMsg != "" {
		statusText = m.statusMsg
	}
	statusBar := ui.StatusBar.Width(m.width).Render(statusText)

	// ── Help hint ──
	keys := []struct{ key, desc string }{
		{"space", "toggle"},
		{"a", "all/none"},
		{"j/k", "navigate"},
		{"enter", "confirm & plan"},
		{"esc", "cancel"},
	}
	var parts []string
	parts = append(parts, ui.StatusKey.Render("[Select]"))
	for _, k := range keys {
		parts = append(parts, ui.HelpKey.Render(k.key)+ui.HelpSep.Render(":")+ui.HelpDesc.Render(k.desc))
	}
	helpHint := strings.Join(parts, " ")

	return lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		content,
		statusBar,
		helpHint,
	)
}

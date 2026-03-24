package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/terraform"
	"github.com/cwar/lazytf/internal/ui"
)

// ─── Key Handling ────────────────────────────────────────

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// --- Overlay dismissal ---
	if m.showHelp || m.showLog || m.showGraph {
		switch key {
		case "q", "esc", "?", "l":
			if m.showHelp && key == "?" {
				m.showHelp = false
			} else if m.showLog && key == "l" {
				m.showLog = false
				m.detailScroll = 0
			} else if key == "esc" || key == "q" {
				m.showHelp = false
				m.showLog = false
				m.showGraph = false
			}
			return m, nil
		case "j", "down":
			m.detailScroll++
			return m, nil
		case "k", "up":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
			return m, nil
		case "d", "ctrl+d":
			m.detailScroll += 15
			return m, nil
		case "u", "ctrl+u":
			m.detailScroll -= 15
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		}
		return m, nil
	}

	// --- Input overlay ---
	if m.showInput {
		return m.handleInputKey(msg)
	}

	// --- Confirm dialog ---
	if m.showConfirm {
		switch key {
		case "y", "Y":
			m.showConfirm = false
			return m.executeConfirmed()
		case "n", "N", "esc", "q":
			m.showConfirm = false
			m.confirmData = ""
			return m, nil
		}
		return m, nil
	}

	// --- Apply/Destroy result: dismiss or scroll ---
	if m.applyResult {
		switch key {
		case "esc", "q":
			m.applyResult = false
			m.onSelectionChanged()
			return m, m.onSelectionChangedCmd()
		case "j", "down":
			m.detailScroll++
			visH := m.detailVisibleHeight()
			max := len(m.detailLines) - visH
			if max < 0 {
				max = 0
			}
			if m.detailScroll > max {
				m.detailScroll = max
			}
			return m, nil
		case "k", "up":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
			return m, nil
		case "g":
			m.detailScroll = 0
			return m, nil
		case "G":
			visH := m.detailVisibleHeight()
			max := len(m.detailLines) - visH
			if max < 0 {
				max = 0
			}
			m.detailScroll = max
			return m, nil
		case "d", "ctrl+d":
			m.detailScroll += 15
			visH := m.detailVisibleHeight()
			max := len(m.detailLines) - visH
			if max < 0 {
				max = 0
			}
			if m.detailScroll > max {
				m.detailScroll = max
			}
			return m, nil
		case "u", "ctrl+u":
			m.detailScroll -= 15
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		case "l":
			// Allow log overlay
		case "?":
			// Allow help overlay
		case "ctrl+c":
			return m, tea.Quit
		default:
			// Swallow other keys to keep the result pinned
			return m, nil
		}
	}

	// --- Plan review: confirm or cancel pending apply/destroy ---
	if m.planReview {
		switch key {
		case "y", "Y":
			planFile := m.pendingPlanFile
			title := "Apply"
			if m.planIsDestroy {
				title = "Destroy"
			}
			// Clear plan review state before starting apply
			m.planReview = false
			m.pendingPlanFile = ""
			m.planIsDestroy = false
			m.planChanges = nil
			m.planFocusView = false
			m.clearLastPlan() // consuming the plan — no recall needed
			return m, m.runTfCmdStream(title, func(ctx context.Context, onLine func(string)) error {
				defer os.Remove(planFile) // clean up plan file after apply
				return m.runner.ApplyPlanStream(ctx, planFile, onLine)
			})
		case "esc":
			m.savePlanState()
			m.statusMsg = ui.DimItem.Render("Plan saved — press R to recall")
			return m, nil
		case "enter", "tab":
			// Toggle focused single-resource view
			if len(m.planChanges) > 0 {
				m.planFocusView = !m.planFocusView
				m.detailScroll = 0
				m.recomputeCompactDiff()
			}
			return m, nil
		case "j", "down":
			if m.planFocusView {
				// Scroll within focused resource block
				max := m.planFocusMaxScroll()
				if m.detailScroll < max {
					m.detailScroll++
				}
			} else if len(m.planChanges) > 0 {
				// Navigate to next resource change
				m.planChangeCur++
				if m.planChangeCur >= len(m.planChanges) {
					m.planChangeCur = 0
				}
				m.detailScroll = m.planChanges[m.planChangeCur].Line
				m.followOutput = false
				m.recomputeCompactDiff()
			}
			return m, nil
		case "k", "up":
			if m.planFocusView {
				// Scroll within focused resource block
				if m.detailScroll > 0 {
					m.detailScroll--
				}
			} else if len(m.planChanges) > 0 {
				// Navigate to previous resource change
				m.planChangeCur--
				if m.planChangeCur < 0 {
					m.planChangeCur = len(m.planChanges) - 1
				}
				m.detailScroll = m.planChanges[m.planChangeCur].Line
				m.followOutput = false
				m.recomputeCompactDiff()
			}
			return m, nil
		case "n":
			// Next resource change (works in both full and focus view)
			if len(m.planChanges) > 0 {
				m.planChangeCur++
				if m.planChangeCur >= len(m.planChanges) {
					m.planChangeCur = 0
				}
				if m.planFocusView {
					m.detailScroll = 0
				} else {
					m.detailScroll = m.planChanges[m.planChangeCur].Line
				}
				m.followOutput = false
				m.recomputeCompactDiff()
			}
			return m, nil
		case "N":
			// Previous resource change (works in both full and focus view)
			if len(m.planChanges) > 0 {
				m.planChangeCur--
				if m.planChangeCur < 0 {
					m.planChangeCur = len(m.planChanges) - 1
				}
				if m.planFocusView {
					m.detailScroll = 0
				} else {
					m.detailScroll = m.planChanges[m.planChangeCur].Line
				}
				m.followOutput = false
				m.recomputeCompactDiff()
			}
			return m, nil
		case "g":
			m.detailScroll = 0
			return m, nil
		case "G":
			max := m.planFocusMaxScroll()
			m.detailScroll = max
			return m, nil
		case "d", "ctrl+d":
			m.detailScroll += 15
			max := m.planFocusMaxScroll()
			if m.detailScroll > max {
				m.detailScroll = max
			}
			return m, nil
		case "u", "ctrl+u":
			m.detailScroll -= 15
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		case "z":
			// Toggle compact diff mode — collapses unchanged heredoc lines
			m.planCompactDiff = !m.planCompactDiff
			m.recomputeCompactDiff()
			m.detailScroll = 0
			return m, nil
		case "c":
			// Copy current resource diff to clipboard
			if len(m.planChanges) > 0 {
				source, _ := m.planFocusBlock()
				text := strings.Join(source, "\n")
				m.statusMsg = ui.DimItem.Render("Copying to clipboard…")
				return m, copyToClipboard(text)
			}
			return m, nil
		}
		// During plan review, only allow a few safe global keys to fall through.
		// Block panel switching, terraform commands, etc. that would overwrite
		// the plan output and leave the user in a broken state.
		switch key {
		case "ctrl+c", "q", "?", "l":
			// These are safe — quit, help overlay, log overlay
		default:
			// Swallow everything else to prevent accidental navigation away
			return m, nil
		}
	}

	// --- Global keys ---
	switch key {
	case "ctrl+c":
		if m.isLoading && m.cancelCmd != nil {
			m.cancelCmd()
			m.statusMsg = ui.WarningStyle.Render("⚠ Cancelling...")
			return m, nil
		}
		return m, tea.Quit
	case "q":
		if m.planReview {
			// Don't quit while reviewing — save plan for later recall
			m.savePlanState()
			m.statusMsg = ui.DimItem.Render("Plan saved — press R to recall")
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "tab":
		if m.focus == FocusLeft {
			m.focus = FocusRight
		} else {
			m.focus = FocusLeft
		}
		return m, nil
	case "l":
		m.showLog = !m.showLog
		m.detailScroll = 0
		return m, nil
	case "R":
		if m.isLoading {
			m.statusMsg = busyMsg()
			return m, nil
		}
		if m.hasLastPlan() {
			if m.restorePlanState() {
				action := "apply"
				if m.planIsDestroy {
					action = "DESTROY"
				}
				m.statusMsg = ui.WarningStyle.Render(fmt.Sprintf("Review plan. Press 'y' to %s, 'esc' to dismiss", action))
				if len(m.planChanges) > 0 {
					m.detailScroll = m.planChanges[0].Line
				}
			} else {
				m.statusMsg = ui.ErrorStyle.Render("Plan file no longer exists")
			}
			return m, nil
		}
		return m, nil
	case "G":
		if m.focus == FocusLeft {
			// graph view
			m.showGraph = true
			m.detailScroll = 0
			if m.graph == nil {
				return m, m.loadGraph()
			}
			m.renderGraphDetail()
			return m, nil
		}
		// Right pane: go to bottom
		visH := m.detailVisibleHeight()
		max := len(m.detailLines) - visH
		if max < 0 {
			max = 0
		}
		m.detailScroll = max
		return m, nil
	}

	// --- Panel switching: number keys and brackets ---
	switch key {
	case "1":
		m.activePanel = PanelStatus
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "2":
		m.activePanel = PanelFiles
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "3":
		m.activePanel = PanelResources
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "4":
		m.activePanel = PanelModules
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "5":
		m.activePanel = PanelWorkspaces
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "6":
		m.activePanel = PanelVarFiles
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "[", "{":
		m.prevPanel()
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "]", "}":
		m.nextPanel()
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	}

	// --- Focus-specific keys ---
	if m.focus == FocusLeft {
		return m.handleLeftKey(key)
	}
	return m.handleRightKey(key)
}

func (m *Model) prevPanel() {
	p := int(m.activePanel) - 1
	if p < 0 {
		p = int(PanelCount) - 1
	}
	m.activePanel = PanelID(p)
	m.focus = FocusLeft
}

func (m *Model) nextPanel() {
	p := int(m.activePanel) + 1
	if p >= int(PanelCount) {
		p = 0
	}
	m.activePanel = PanelID(p)
	m.focus = FocusLeft
}

func (m Model) handleLeftKey(key string) (tea.Model, tea.Cmd) {
	panel := m.panels[m.activePanel]

	// Navigation
	switch key {
	case "j", "down":
		panel.MoveDown()
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "k", "up":
		panel.MoveUp()
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "enter", " ":
		return m.handlePanelAction()
	}

	// Context-specific keys (panel-dependent actions like edit, taint, etc.)
	if m.isLoading && isContextOperationKey(m.activePanel, key) {
		m.statusMsg = busyMsg()
		return m, nil
	}
	if newM, cmd, handled := m.handleContextKey(key); handled {
		return newM, cmd
	}

	// Global terraform commands (work from any panel)
	return m.runGlobalCommand(key)
}

func (m Model) handleRightKey(key string) (tea.Model, tea.Cmd) {
	visH := m.detailVisibleHeight()
	maxScroll := len(m.detailLines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch key {
	case "j", "down":
		if m.detailScroll < maxScroll {
			m.detailScroll++
		}
		// Re-engage follow if scrolled to bottom
		if m.detailScroll >= maxScroll {
			m.followOutput = true
		}
	case "k", "up":
		if m.detailScroll > 0 {
			m.detailScroll--
			m.followOutput = false // user scrolled up, stop auto-following
		}
	case "d", "ctrl+d":
		m.detailScroll += 15
		if m.detailScroll > maxScroll {
			m.detailScroll = maxScroll
		}
		if m.detailScroll >= maxScroll {
			m.followOutput = true
		}
	case "u", "ctrl+u":
		m.detailScroll -= 15
		if m.detailScroll < 0 {
			m.detailScroll = 0
		}
		m.followOutput = false
	case "g":
		m.detailScroll = 0
		m.followOutput = false
	case "G":
		m.detailScroll = maxScroll
		m.followOutput = true

	// Context key: edit the file currently being viewed
	case "e":
		return m.handleRightPaneEdit()
	}

	// Global terraform commands also work from the right pane
	return m.runGlobalCommand(key)
}

// runGlobalCommand dispatches terraform commands that work from any context
// (left panels or right detail pane). Returns (m, nil) if the key is not
// a recognised command.
func (m Model) runGlobalCommand(key string) (tea.Model, tea.Cmd) {
	if m.isLoading && isOperationKey(key) {
		m.statusMsg = busyMsg()
		return m, nil
	}
	switch key {
	case "p":
		return m, m.runTfCmdStream("Plan", func(ctx context.Context, onLine func(string)) error {
			return m.runner.PlanStream(ctx, m.selectedVarFile, onLine)
		})
	case "a":
		m.clearLastPlan()
		planFile := tempPlanFile()
		m.pendingPlanFile = planFile
		m.planIsDestroy = false
		varFile := m.selectedVarFile
		return m, m.runTfCmdStream("Plan → Apply", func(ctx context.Context, onLine func(string)) error {
			return m.runner.PlanSaveStream(ctx, varFile, planFile, false, onLine)
		})
	case "i":
		return m, m.runTfCmd("Init", func() (string, error) {
			return m.runner.Init()
		})
	case "v":
		return m, m.runTfCmd("Validate", func() (string, error) {
			return m.runner.Validate()
		})
	case "f":
		return m, m.runTfCmd("Format Check", func() (string, error) {
			return m.runner.Fmt()
		})
	case "F":
		return m, m.runTfCmd("Format Fix", func() (string, error) {
			return m.runner.FmtFix()
		})
	case "D":
		m.clearLastPlan()
		planFile := tempPlanFile()
		m.pendingPlanFile = planFile
		m.planIsDestroy = true
		varFile := m.selectedVarFile
		return m, m.runTfCmdStream("Plan → Destroy", func(ctx context.Context, onLine func(string)) error {
			return m.runner.PlanSaveStream(ctx, varFile, planFile, true, onLine)
		})
	case "P":
		return m, m.runTfCmd("Providers", func() (string, error) {
			return m.runner.Providers()
		})
	case "r":
		m.statusMsg = ui.SpinnerLabel.Render("⟳ Refreshing...")
		return m, m.loadAllData()
	}
	return m, nil
}

// handlePanelAction processes enter/space on the active panel.
func (m Model) handlePanelAction() (tea.Model, tea.Cmd) {
	panel := m.panels[m.activePanel]
	item := panel.SelectedItem()
	if item == nil {
		return m, nil
	}

	switch m.activePanel {
	case PanelWorkspaces:
		// Switch workspace — reset to auto var-file selection
		wsName := item.Label
		if wsName != m.workspace {
			if m.isLoading {
				m.statusMsg = busyMsg()
				return m, nil
			}
			m.varFileManual = false
			return m, m.runTfCmd("Workspace: "+wsName, func() (string, error) {
				return m.runner.WorkspaceSelect(wsName)
			})
		}

	case PanelVarFiles:
		// Toggle var file selection (manual override)
		if f, ok := item.Data.(terraform.TfFile); ok {
			if m.selectedVarFile == f.Path && m.varFileManual {
				// Deselect manual → revert to auto
				m.varFileManual = false
				m.autoSelectVarFile()
			} else {
				m.selectedVarFile = f.Path
				m.varFileManual = true
			}
			m.rebuildVarFilesPanel()
		}

	case PanelResources:
		// Show resource detail (enter focuses the right pane)
		m.focus = FocusRight
		return m, nil

	case PanelFiles:
		// Enter focuses the right pane to scroll the file
		m.focus = FocusRight
		return m, nil

	case PanelModules:
		// Enter focuses the right pane
		m.focus = FocusRight
		return m, nil
	}

	return m, nil
}

func (m Model) executeConfirmed() (Model, tea.Cmd) {
	if m.isLoading {
		m.statusMsg = busyMsg()
		return m, nil
	}

	action := m.confirmAction
	data := m.confirmData
	m.confirmAction = ""
	m.confirmData = ""
	m.confirmMsg = ""

	switch action {
	case "state_rm":
		return m, m.runTfCmd("State Rm: "+data, func() (string, error) {
			return m.runner.StateRm(data)
		})
	case "workspace_delete":
		return m, m.runTfCmd("Delete Workspace: "+data, func() (string, error) {
			return m.runner.WorkspaceDelete(data)
		})
	}

	return m, nil
}

// planFocusBlock returns the detail lines and highlighted lines for just the
// currently selected resource change block. Used in focused view mode.
func (m Model) planFocusBlock() (source []string, hl []string) {
	if len(m.planChanges) == 0 {
		return m.detailLines, m.highlightedLines
	}
	c := m.planChanges[m.planChangeCur]
	start := c.Line
	end := c.EndLine
	if start > len(m.detailLines) {
		start = len(m.detailLines)
	}
	if end > len(m.detailLines) {
		end = len(m.detailLines)
	}
	source = m.detailLines[start:end]
	if m.isHighlighted && len(m.highlightedLines) >= end {
		hl = m.highlightedLines[start:end]
	}
	return source, hl
}

// planViewLines returns the lines and highlighted lines for the current plan
// view, applying compact diff if enabled. Handles both full plan and focused
// single-resource views.
func (m *Model) planViewLines() (source []string, hl []string) {
	if m.planFocusView && len(m.planChanges) > 0 {
		source, hl = m.planFocusBlock()
	} else {
		source = m.detailLines
		hl = m.highlightedLines
	}

	if !m.planCompactDiff {
		return source, hl
	}

	// Use precomputed compact lines (computed in recomputeCompactDiff)
	if m.compactLines != nil {
		return m.compactLines, m.compactHighlighted
	}
	return source, hl
}

// recomputeCompactDiff rebuilds the compact diff cache from current state.
// Must be called in Update() whenever planCompactDiff, planFocusView,
// planChangeCur, or the underlying plan data changes.
func (m *Model) recomputeCompactDiff() {
	if !m.planCompactDiff {
		m.compactLines = nil
		m.compactHighlighted = nil
		return
	}

	var source, hl []string
	if m.planFocusView && len(m.planChanges) > 0 {
		source, hl = m.planFocusBlock()
	} else {
		source = m.detailLines
		hl = m.highlightedLines
	}

	if len(hl) == len(source) {
		m.compactLines, m.compactHighlighted = ui.CompactDiffHighlighted(source, hl, 3)
	} else {
		m.compactLines = ui.CompactDiff(source, 3)
		m.compactHighlighted = nil
	}
}

// planFocusMaxScroll returns the max scroll value for the current view
// (focused block or full plan), accounting for compact diff.
func (m *Model) planFocusMaxScroll() int {
	visH := m.detailVisibleHeight()
	if visH < 1 {
		visH = 1
	}
	source, _ := m.planViewLines()
	total := len(source)
	max := total - visH
	if max < 0 {
		max = 0
	}
	return max
}

// detailVisibleHeight returns the number of content lines visible in the
// detail pane. The layout is: contentH = m.height - 2 (status + help),
// and renderDetailPane uses 2 lines for the title bar and bottom padding,
// leaving contentH - 2 = m.height - 4 visible lines.
func (m *Model) detailVisibleHeight() int {
	h := m.height - 4
	if h < 1 {
		return 1
	}
	return h
}

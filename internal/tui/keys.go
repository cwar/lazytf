package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/terraform"
	"github.com/cwar/lazytf/internal/ui"
)

// ─── Key Handling ────────────────────────────────────────

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// --- Multi-workspace mode ---
	if m.multiWS.active {
		return m.handleMultiWSKey(msg)
	}

	// --- Overlay dismissal (help, graph) ---
	if m.showHelp || m.showGraph {
		max := m.detailMaxScroll()
		switch key {
		case "q", "esc", "?":
			if m.showHelp && key == "?" {
				m.showHelp = false
			} else if key == "esc" || key == "q" {
				m.showHelp = false
				m.showGraph = false
			}
			return m, nil
		case "j", "down":
			m.scrollDown(max)
			return m, nil
		case "k", "up":
			m.scrollUp()
			return m, nil
		case "d", "ctrl+d":
			m.scrollPageDown(max)
			return m, nil
		case "u", "ctrl+u":
			m.scrollPageUp()
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
		max := m.detailMaxScroll()
		switch key {
		case "esc", "q":
			m.applyResult = false
			m.onSelectionChanged()
			return m, m.onSelectionChangedCmd()
		case "j", "down":
			m.scrollDown(max)
			return m, nil
		case "k", "up":
			m.scrollUp()
			return m, nil
		case "g":
			m.scrollToTop()
			return m, nil
		case "G":
			m.scrollToBottom(max)
			return m, nil
		case "d", "ctrl+d":
			m.scrollPageDown(max)
			return m, nil
		case "u", "ctrl+u":
			m.scrollPageUp()
			return m, nil
		case "l":
			// Allow jumping to history panel
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
			m.planView.Reset()
			m.clearLastPlan() // consuming the plan — no recall needed
			return m, m.runTfCmdStream(title, func(ctx context.Context, onLine func(string)) error {
				defer os.Remove(planFile) // clean up plan file after apply
				return m.runner.ApplyPlanStream(ctx, planFile, onLine)
			})
		case "esc":
			m.savePlanState()
			m.statusMsg = ui.DimItem.Render("Plan saved — press R to recall")
			return m, nil
		case "f":
			// Toggle focused single-resource view
			if m.planView.ToggleFocus(m.planChanges) {
				m.detailScroll = 0
				m.planView.RecomputeCompact(m.detailLines, m.highlightedLines, m.planChanges)
			}
			return m, nil
		case "j", "down":
			if m.planView.focusView {
				// Scroll within focused resource block
				max := m.planView.MaxScroll(m.detailLines, m.highlightedLines, m.planChanges, m.detailVisibleHeight())
				if m.detailScroll < max {
					m.detailScroll++
				}
			} else if m.planView.NextChange(m.planChanges) {
				// Navigate to next resource change
				m.detailScroll = m.planChanges[m.planView.changeCur].Line
				m.followOutput = false
				m.planView.RecomputeCompact(m.detailLines, m.highlightedLines, m.planChanges)
			}
			return m, nil
		case "k", "up":
			if m.planView.focusView {
				// Scroll within focused resource block
				if m.detailScroll > 0 {
					m.detailScroll--
				}
			} else if m.planView.PrevChange(m.planChanges) {
				// Navigate to previous resource change
				m.detailScroll = m.planChanges[m.planView.changeCur].Line
				m.followOutput = false
				m.planView.RecomputeCompact(m.detailLines, m.highlightedLines, m.planChanges)
			}
			return m, nil
		case "n":
			// Next resource change (works in both full and focus view)
			if m.planView.NextChange(m.planChanges) {
				if m.planView.focusView {
					m.detailScroll = 0
				} else {
					m.detailScroll = m.planChanges[m.planView.changeCur].Line
				}
				m.followOutput = false
				m.planView.RecomputeCompact(m.detailLines, m.highlightedLines, m.planChanges)
			}
			return m, nil
		case "N":
			// Previous resource change (works in both full and focus view)
			if m.planView.PrevChange(m.planChanges) {
				if m.planView.focusView {
					m.detailScroll = 0
				} else {
					m.detailScroll = m.planChanges[m.planView.changeCur].Line
				}
				m.followOutput = false
				m.planView.RecomputeCompact(m.detailLines, m.highlightedLines, m.planChanges)
			}
			return m, nil
		case "g":
			m.scrollToTop()
			return m, nil
		case "G":
			m.scrollToBottom(m.planView.MaxScroll(m.detailLines, m.highlightedLines, m.planChanges, m.detailVisibleHeight()))
			return m, nil
		case "d", "ctrl+d":
			m.scrollPageDown(m.planView.MaxScroll(m.detailLines, m.highlightedLines, m.planChanges, m.detailVisibleHeight()))
			return m, nil
		case "u", "ctrl+u":
			m.scrollPageUp()
			return m, nil
		case "z":
			// Toggle compact diff mode — collapses unchanged heredoc lines
			m.planView.compactDiff = !m.planView.compactDiff
			m.planView.RecomputeCompact(m.detailLines, m.highlightedLines, m.planChanges)
			m.detailScroll = 0
			return m, nil
		case "c":
			// Copy current resource diff to clipboard
			if len(m.planChanges) > 0 {
				text := m.planView.CopyBlock(m.detailLines, m.planChanges)
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
	case "b":
		// Switch back to streaming command output (when a command is running
		// and user navigated away to browse files/resources).
		if m.isLoading && !m.viewingStream && len(m.streamLines) > 0 {
			m.detailLines = m.streamLines
			m.highlightedLines = m.streamHLLines
			m.isHighlighted = true
			// Restore the command title (strip trailing "..." from loadingMsg)
			title := m.loadingMsg
			if len(title) > 3 && title[len(title)-3:] == "..." {
				title = title[:len(title)-3]
			}
			m.detailTitle = title
			m.viewingStream = true
			m.followOutput = true
			m.focus = FocusRight
			// Scroll to bottom to follow live output
			visH := m.detailVisibleHeight()
			if len(m.detailLines) > visH {
				m.detailScroll = len(m.detailLines) - visH
			} else {
				m.detailScroll = 0
			}
			return m, nil
		}
	case "tab":
		m.nextPanel()
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "shift+tab":
		m.prevPanel()
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "l":
		// Jump to command history panel
		m.activePanel = PanelHistory
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
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
		m.scrollToBottom(m.detailMaxScroll())
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
		m.activePanel = PanelWorkspaces
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "5":
		m.activePanel = PanelVarFiles
		m.focus = FocusLeft
		m.onSelectionChanged()
		return m, m.onSelectionChangedCmd()
	case "6":
		m.activePanel = PanelHistory
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
	max := m.detailMaxScroll()

	switch key {
	case "j", "down":
		m.scrollDown(max)
		if m.detailScroll >= max {
			m.followOutput = true
		}
	case "k", "up":
		m.scrollUp()
		m.followOutput = false
	case "d", "ctrl+d":
		m.scrollPageDown(max)
		if m.detailScroll >= max {
			m.followOutput = true
		}
	case "u", "ctrl+u":
		m.scrollPageUp()
		m.followOutput = false
	case "g":
		m.scrollToTop()
		m.followOutput = false
	case "G":
		m.scrollToBottom(max)
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
		// Init runs interactively via tea.ExecProcess so Git credential
		// prompts, SSH agent dialogs, and other auth flows work. The TUI
		// suspends, terraform gets the real terminal, and we resume + reload
		// data when it exits.
		cmd := exec.Command(m.runner.Binary, "init")
		cmd.Dir = m.runner.WorkDir
		m.statusMsg = ui.SpinnerLabel.Render("⟳ Running init (switched to terminal)…")
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return initFinishedMsg{err: err}
		})
	case "v":
		return m, m.runTfCmd("Validate", func(ctx context.Context) (string, error) {
			return m.runner.RunCtx(ctx, "validate")
		})
	case "f":
		return m, m.runTfCmd("Format Check", func(ctx context.Context) (string, error) {
			return m.runner.RunCtx(ctx, "fmt", "-check", "-diff")
		})
	case "F":
		return m, m.runTfCmd("Format Fix", func(ctx context.Context) (string, error) {
			return m.runner.RunCtx(ctx, "fmt")
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
		return m, m.runTfCmd("Providers", func(ctx context.Context) (string, error) {
			return m.runner.RunCtx(ctx, "providers")
		})
	case "r":
		m.statusMsg = ui.SpinnerLabel.Render("⟳ Refreshing...")
		return m, m.loadAllData()
	case "W":
		// Multi-workspace mode: prompt for filter
		m.showInput = true
		m.inputPrompt = "Multi-workspace plan — filter (blank=all):"
		m.inputValue = ""
		m.inputAction = "multi_ws_plan"
		return m, nil
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
		wsName, _ := item.Data.(string)
		if wsName != "" && wsName != m.workspace {
			if m.isLoading {
				m.statusMsg = busyMsg()
				return m, nil
			}
			m.varFileManual = false
			return m, m.runTfCmd("Workspace: "+wsName, func(ctx context.Context) (string, error) {
				return m.runner.RunCtx(ctx, "workspace", "select", wsName)
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

	case PanelHistory:
		// Enter focuses the right pane to scroll command output
		m.focus = FocusRight
		return m, nil
	}

	return m, nil
}

func (m Model) executeConfirmed() (tea.Model, tea.Cmd) {
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
		return m, m.runTfCmd("State Rm: "+data, func(ctx context.Context) (string, error) {
			return m.runner.RunCtx(ctx, "state", "rm", data)
		})
	case "workspace_delete":
		return m, m.runTfCmd("Delete Workspace: "+data, func(ctx context.Context) (string, error) {
			return m.runner.RunCtx(ctx, "workspace", "delete", data)
		})
	}

	return m, nil
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

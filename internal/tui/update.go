package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/terraform"
	"github.com/cwar/lazytf/internal/ui"
)

// ─── Update ──────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case cmdStartMsg:
		m.isLoading = true
		m.loadingMsg = msg.title + "..."
		m.detailTitle = msg.title
		m.detailLines = nil
		m.highlightedLines = nil
		m.isHighlighted = true
		m.detailScroll = 0
		m.followOutput = true
		m.applyResult = false
		m.planHighlighter = ui.NewPlanHighlighter()
		m.streamLines = nil
		m.streamHLLines = nil
		m.viewingStream = true
		m.focus = FocusRight // auto-focus detail pane to allow scrolling
		return m, nil

	case cmdStreamLineMsg:
		// Always buffer the streaming output — this survives navigation.
		hlLine := m.planHighlighter.HighlightLine(msg.line)
		m.streamLines = append(m.streamLines, msg.line)
		m.streamHLLines = append(m.streamHLLines, hlLine)

		// Only update the display if the user is viewing the stream.
		// If they navigated to a file, don't corrupt detailLines.
		if m.viewingStream {
			m.detailLines = append(m.detailLines, msg.line)
			m.highlightedLines = append(m.highlightedLines, hlLine)
			// Auto-scroll to follow output (unless user scrolled up)
			if m.followOutput {
				visH := m.detailVisibleHeight()
				if visH < 1 {
					visH = 1
				}
				if len(m.detailLines) > visH {
					m.detailScroll = len(m.detailLines) - visH
				}
			}
		}
		return m, readStreamLine(msg.title, msg.ch, msg.cmdErr)

	case dataLoadedMsg:
		m.files = msg.files
		m.resources = msg.resources
		m.modules = msg.modules
		m.workspaces = msg.workspaces
		m.outputs = msg.outputs
		m.gitBranch = msg.gitBranch
		m.resourceIndex = msg.resourceIndex
		if m.workspaces != nil {
			m.workspace = m.workspaces.Current
		}
		m.fileTree = terraform.BuildFileTree(m.files)
		m.autoSelectVarFile()
		m.rebuildAllPanels()

		// Surface any load errors in the status bar
		if len(msg.errors) > 0 {
			m.statusMsg = ui.ErrorStyle.Render("⚠ Load errors: " + strings.Join(msg.errors, "; "))
		}

		// Don't overwrite the detail pane if we're showing apply/destroy output
		if !m.applyResult {
			m.onSelectionChanged()
			return m, m.onSelectionChangedCmd()
		}
		return m, nil

	case stateShowMsg:
		// Only update if we're still looking at this resource
		if m.detailTitle == msg.address || m.detailTitle == "Loading: "+msg.address {
			m.detailTitle = msg.address
			m.setDetailContent(msg.output, true)
		}
		return m, nil

	case graphLoadedMsg:
		m.graph = msg.graph
		if m.showGraph {
			m.renderGraphDetail()
		}
		return m, nil

	case editorFinishedMsg:
		// Reload data after editor closes (files may have changed)
		if msg.err != nil {
			m.statusMsg = ui.ErrorStyle.Render("Editor: " + msg.err.Error())
		}
		return m, m.loadAllData()

	case initFinishedMsg:
		// Reload data after init (providers, modules, state may have changed)
		if msg.err != nil {
			m.statusMsg = ui.ErrorStyle.Render("Init failed — check output above")
		} else {
			m.statusMsg = ui.SuccessStyle.Render("✓ Init complete")
		}
		return m, m.loadAllData()

	case multiWSPlanDoneMsg:
		m.handleMultiWSPlanDone(msg)
		return m, nil

	case multiWSApplyLineMsg:
		m.handleMultiWSApplyLine(msg)
		return m, readMultiWSApplyLine(msg.workspace, msg.ch, msg.cmdErr)

	case multiWSApplyDoneMsg:
		m.handleMultiWSApplyDone(msg)
		cmd := m.startNextMultiWSApply()
		return m, cmd

	case clipboardMsg:
		if msg.err != nil {
			m.statusMsg = ui.ErrorStyle.Render("Clipboard: " + msg.err.Error())
		} else {
			addr := ""
			if c := m.planView.CurrentChange(m.planChanges); c != nil {
				addr = " — " + c.Address
			}
			m.statusMsg = ui.SuccessStyle.Render("✓ Copied to clipboard" + addr)
		}
		return m, nil

	case cmdDoneMsg:
		m.isLoading = false
		m.loadingMsg = ""
		m.followOutput = false
		m.cancelCmd = nil

		if msg.err != nil {
			m.statusMsg = ui.ErrorStyle.Render("✗ " + msg.title + " failed")
		} else {
			m.statusMsg = ui.SuccessStyle.Render("✓ " + msg.title + " complete")
		}
		m.detailTitle = msg.title

		if msg.streamed {
			// Restore from the stream buffer — it's the authoritative source.
			// The user may have navigated away, which replaced detailLines
			// with file content. The buffer always has the full stream.
			if len(m.streamLines) > 0 {
				m.detailLines = m.streamLines
			}
			m.viewingStream = false // buffer consumed — no longer "streaming"

			// Re-highlight the full output for better plan summary rendering.
			fullOutput := strings.Join(m.detailLines, "\n")
			m.highlightedLines = ui.HighlightPlanOutput(fullOutput)
			m.isHighlighted = true
			// Log it
			m.cmdOutput = append(m.cmdOutput, "─── "+msg.title+" ───")
			m.cmdOutput = append(m.cmdOutput, m.detailLines...)
		} else {
			// Non-streamed command: set detail from output
			isPlan := strings.Contains(msg.title, "Plan") ||
				strings.Contains(msg.title, "Apply") ||
				strings.Contains(msg.title, "Destroy")
			if isPlan {
				m.detailLines = strings.Split(msg.output, "\n")
				m.highlightedLines = ui.HighlightPlanOutput(msg.output)
				m.isHighlighted = true
			} else {
				m.setDetailContent(msg.output, false)
			}
			m.detailScroll = 0
			// Log it
			m.cmdOutput = append(m.cmdOutput, "─── "+msg.title+" ───")
			m.cmdOutput = append(m.cmdOutput, strings.Split(msg.output, "\n")...)
		}

		// Plan review: if a plan file was saved, enter review mode
		if m.pendingPlanFile != "" && !m.planReview {
			if msg.err != nil {
				// Plan failed — clean up the plan file but pin the output
				// so the user can read the error. Same pattern as applyResult.
				os.Remove(m.pendingPlanFile)
				m.pendingPlanFile = ""
				m.planIsDestroy = false
				m.applyResult = true
				m.statusMsg = ui.ErrorStyle.Render("✗ " + msg.title + " failed — press esc to dismiss")
			} else {
				// Plan succeeded — enter review mode
				m.planReview = true
				m.planChanges = parsePlanChanges(m.detailLines)
				m.planView.Reset()
				action := "apply"
				if m.planIsDestroy {
					action = "DESTROY"
				}
				m.statusMsg = ui.WarningStyle.Render(fmt.Sprintf("Review plan. Press 'y' to %s, 'esc' to cancel", action))
				if len(m.planChanges) > 0 {
					// Scroll to first change
					m.detailScroll = m.planChanges[0].Line
				} else {
					m.detailScroll = 0
				}
				return m, nil // don't reload data yet
			}
		}

		// Apply/Destroy result: pin the output so it stays visible after
		// data reload. The user needs to see what happened.
		if msg.title == "Apply" || msg.title == "Destroy" {
			m.applyResult = true
			if msg.err != nil {
				m.statusMsg = ui.ErrorStyle.Render("✗ " + msg.title + " failed — press esc to dismiss")
			} else {
				m.statusMsg = ui.SuccessStyle.Render("✓ " + msg.title + " complete — press esc to dismiss")
			}
		}

		// Workspace switch: eagerly update workspace + var file so that any
		// command the user fires before dataLoadedMsg arrives uses the correct
		// var file. Without this, there's a race window where pressing 'a'
		// right after switching workspaces would plan with the OLD var file.
		if strings.HasPrefix(msg.title, "Workspace: ") && msg.err == nil {
			newWs := strings.TrimPrefix(msg.title, "Workspace: ")
			m.workspace = newWs
			m.wsFilter = "" // clear workspace filter after successful switch
			m.autoSelectVarFile()
		}

		// Reload data
		return m, m.loadAllData()
	}

	return m, nil
}

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cwar/lazytf/internal/terraform"
	"github.com/cwar/lazytf/internal/ui"
)

// editorFinishedMsg is sent when an external editor process exits.
type editorFinishedMsg struct{ err error }

// keyHint represents a key-description pair for help display.
type keyHint struct {
	Key  string
	Desc string
}

// ─── Context Key Dispatch ────────────────────────────────

// handleContextKey dispatches to the appropriate panel-specific key handler.
// Returns (model, cmd, handled). If handled is false, the caller should
// fall through to global key handling.
func (m Model) handleContextKey(key string) (tea.Model, tea.Cmd, bool) {
	switch m.activePanel {
	case PanelFiles:
		return m.handleFilesContextKey(key)
	case PanelResources:
		return m.handleResourcesContextKey(key)
	case PanelModules:
		return m.handleModulesContextKey(key)
	case PanelWorkspaces:
		return m.handleWorkspacesContextKey(key)
	case PanelVarFiles:
		return m.handleVarFilesContextKey(key)
	}
	return m, nil, false
}

// ─── Files Panel ─────────────────────────────────────────

func (m Model) handleFilesContextKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "e":
		panel := m.panels[PanelFiles]
		if item := panel.SelectedItem(); item != nil {
			if f, ok := item.Data.(terraform.TfFile); ok {
				return m, m.openEditor(f.Path, 0), true
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

// ─── Resources Panel ─────────────────────────────────────

func (m Model) handleResourcesContextKey(key string) (tea.Model, tea.Cmd, bool) {
	panel := m.panels[PanelResources]

	switch key {
	case "e":
		// Jump to the file where this resource is declared
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				path, line := m.findResourceFile(r.Address)
				if path != "" {
					return m, m.openEditor(path, line), true
				}
				m.statusMsg = ui.WarningStyle.Render("Could not find declaration for " + r.Address)
				return m, nil, true
			}
		}
		return m, nil, true

	case "s":
		// Force refresh state show and focus right pane
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				m.focus = FocusRight
				m.detailTitle = "Loading: " + r.Address
				m.setDetailPlain("Loading state for " + r.Address + "...")
				return m, m.loadStateShow(r.Address), true
			}
		}
		return m, nil, true

	case "t":
		// Taint resource
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				return m, m.runTfCmd("Taint: "+r.Address, func() (string, error) {
					return m.runner.StateTaint(r.Address)
				}), true
			}
		}
		return m, nil, true

	case "u":
		// Untaint resource
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				return m, m.runTfCmd("Untaint: "+r.Address, func() (string, error) {
					return m.runner.StateUntaint(r.Address)
				}), true
			}
		}
		return m, nil, true

	case "x":
		// State rm with confirmation
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				m.showConfirm = true
				m.confirmAction = "state_rm"
				m.confirmData = r.Address
				m.confirmMsg = fmt.Sprintf(
					"Remove '%s' from state?\n\n"+
						"This will NOT destroy the actual resource —\n"+
						"it only removes terraform's tracking of it.",
					r.Address)
				return m, nil, true
			}
		}
		return m, nil, true

	case "T":
		// Targeted plan → review → apply
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				m.clearLastPlan() // new plan replaces any saved plan
				planFile := filepath.Join(os.TempDir(), fmt.Sprintf("lazytf-%d.tfplan", os.Getpid()))
				m.pendingPlanFile = planFile
				m.planIsDestroy = false
				varFile := m.selectedVarFile
				target := r.Address
				return m, m.runTfCmdStream("Targeted Plan: "+r.Address, func(onLine func(string)) error {
					return m.runner.PlanTargetSaveStream(varFile, planFile, []string{target}, onLine)
				}), true
			}
		}
		return m, nil, true
	}

	return m, nil, false
}

// ─── Modules Panel ───────────────────────────────────────

func (m Model) handleModulesContextKey(key string) (tea.Model, tea.Cmd, bool) {
	panel := m.panels[PanelModules]

	switch key {
	case "e":
		// Open the file where this module is declared
		if item := panel.SelectedItem(); item != nil {
			if mod, ok := item.Data.(terraform.ModuleCall); ok {
				if mod.SourceFile != "" {
					path := mod.SourceFile
					if !filepath.IsAbs(path) {
						path = filepath.Join(m.workDir, path)
					}
					return m, m.openEditor(path, 0), true
				}
			}
		}
		return m, nil, true

	case "o":
		// Open module directory in editor
		if item := panel.SelectedItem(); item != nil {
			if mod, ok := item.Data.(terraform.ModuleCall); ok {
				dir := mod.ModuleDir(m.workDir)
				if dir != "" {
					absDir := dir
					if !filepath.IsAbs(dir) {
						absDir = filepath.Join(m.workDir, dir)
					}
					return m, m.openEditor(absDir, 0), true
				}
				m.statusMsg = ui.WarningStyle.Render("Module '" + mod.Name + "' has no local directory")
				return m, nil, true
			}
		}
		return m, nil, true

	case "T":
		// Targeted plan for module
		if item := panel.SelectedItem(); item != nil {
			if mod, ok := item.Data.(terraform.ModuleCall); ok {
				m.clearLastPlan() // new plan replaces any saved plan
				target := "module." + mod.Name
				planFile := filepath.Join(os.TempDir(), fmt.Sprintf("lazytf-%d.tfplan", os.Getpid()))
				m.pendingPlanFile = planFile
				m.planIsDestroy = false
				varFile := m.selectedVarFile
				return m, m.runTfCmdStream("Targeted Plan: "+target, func(onLine func(string)) error {
					return m.runner.PlanTargetSaveStream(varFile, planFile, []string{target}, onLine)
				}), true
			}
		}
		return m, nil, true
	}

	return m, nil, false
}

// ─── Workspaces Panel ────────────────────────────────────

func (m Model) handleWorkspacesContextKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "n":
		// New workspace — show input prompt
		m.showInput = true
		m.inputPrompt = "New Workspace Name"
		m.inputValue = ""
		m.inputAction = "workspace_new"
		return m, nil, true

	case "x":
		// Delete workspace
		panel := m.panels[PanelWorkspaces]
		if item := panel.SelectedItem(); item != nil {
			wsName := item.Label
			if wsName == m.workspace {
				m.statusMsg = ui.ErrorStyle.Render("Cannot delete the active workspace — switch first")
				return m, nil, true
			}
			m.showConfirm = true
			m.confirmAction = "workspace_delete"
			m.confirmData = wsName
			m.confirmMsg = fmt.Sprintf("Delete workspace '%s'?\n\nThis cannot be undone.", wsName)
			return m, nil, true
		}
		return m, nil, true
	}

	return m, nil, false
}

// ─── Var Files Panel ─────────────────────────────────────

func (m Model) handleVarFilesContextKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "e":
		panel := m.panels[PanelVarFiles]
		if item := panel.SelectedItem(); item != nil {
			if f, ok := item.Data.(terraform.TfFile); ok {
				return m, m.openEditor(f.Path, 0), true
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

// ─── Right Pane Edit ─────────────────────────────────────

// handleRightPaneEdit opens the file currently being viewed in the detail
// pane, based on which left panel is active.
func (m Model) handleRightPaneEdit() (tea.Model, tea.Cmd) {
	switch m.activePanel {
	case PanelFiles:
		panel := m.panels[PanelFiles]
		if item := panel.SelectedItem(); item != nil {
			if f, ok := item.Data.(terraform.TfFile); ok {
				return m, m.openEditor(f.Path, 0)
			}
		}
	case PanelVarFiles:
		panel := m.panels[PanelVarFiles]
		if item := panel.SelectedItem(); item != nil {
			if f, ok := item.Data.(terraform.TfFile); ok {
				return m, m.openEditor(f.Path, 0)
			}
		}
	case PanelResources:
		panel := m.panels[PanelResources]
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				path, line := m.findResourceFile(r.Address)
				if path != "" {
					return m, m.openEditor(path, line)
				}
			}
		}
	case PanelModules:
		panel := m.panels[PanelModules]
		if item := panel.SelectedItem(); item != nil {
			if mod, ok := item.Data.(terraform.ModuleCall); ok {
				if mod.SourceFile != "" {
					path := mod.SourceFile
					if !filepath.IsAbs(path) {
						path = filepath.Join(m.workDir, path)
					}
					return m, m.openEditor(path, 0)
				}
			}
		}
	}
	return m, nil
}

// ─── Input Overlay ───────────────────────────────────────

func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		if m.inputValue != "" {
			return m.submitInput()
		}
		return m, nil
	case "esc":
		m.showInput = false
		m.inputValue = ""
		return m, nil
	case "backspace", "ctrl+h":
		if len(m.inputValue) > 0 {
			m.inputValue = m.inputValue[:len(m.inputValue)-1]
		}
		return m, nil
	case "ctrl+u":
		m.inputValue = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	default:
		// Accept workspace-safe characters: a-z, A-Z, 0-9, -, _
		if len(key) == 1 {
			c := key[0]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') || c == '-' || c == '_' {
				m.inputValue += key
			}
		}
		return m, nil
	}
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	value := m.inputValue
	action := m.inputAction
	m.showInput = false
	m.inputValue = ""
	m.inputAction = ""

	if m.isLoading {
		m.statusMsg = busyMsg()
		return m, nil
	}

	switch action {
	case "workspace_new":
		return m, m.runTfCmd("New Workspace: "+value, func() (string, error) {
			return m.runner.WorkspaceNew(value)
		})
	}
	return m, nil
}

func (m Model) renderInput() string {
	prompt := ui.Logo.Render("⚡ " + m.inputPrompt)

	cursor := "█"
	value := m.inputValue + cursor

	var lines []string
	lines = append(lines, prompt)
	lines = append(lines, "")
	lines = append(lines, "  "+value)
	lines = append(lines, "")
	lines = append(lines, "  "+ui.HelpKey.Render("enter")+" confirm    "+ui.HelpKey.Render("esc")+" cancel")

	content := strings.Join(lines, "\n")
	w := 50
	if w > m.width-4 {
		w = m.width - 4
	}

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Purple).
			Padding(1, 2).
			Width(w).
			Render(content),
	)
}

// ─── Editor ──────────────────────────────────────────────

// resolveEditor returns the user's preferred editor from $EDITOR/$VISUAL,
// falling back to vi.
func resolveEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	return "vi"
}

// openEditor launches the user's editor for the given file, optionally at a
// specific line number. Uses tea.ExecProcess to suspend the TUI, hand terminal
// control to the editor, and resume when it exits.
func (m *Model) openEditor(file string, line int) tea.Cmd {
	editor := resolveEditor()
	var cmd *exec.Cmd
	if line > 0 {
		cmd = exec.Command(editor, fmt.Sprintf("+%d", line), file)
	} else {
		cmd = exec.Command(editor, file)
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// ─── Resource File Finding ───────────────────────────────

// findResourceFile searches .tf files for the HCL declaration of a resource
// given its state address (e.g., "aws_instance.example",
// "module.vpc.aws_subnet.public", or "data.aws_ami.ubuntu").
// Returns the file path and 1-indexed line number, or ("", 0) if not found.
func (m *Model) findResourceFile(address string) (string, int) {
	parts := strings.Split(address, ".")

	// Strip module prefix: "module.name" pairs
	i := 0
	for i < len(parts)-2 {
		if parts[i] == "module" {
			i += 2
		} else {
			break
		}
	}

	remaining := parts[i:]

	var keyword, resType, resName string
	if len(remaining) >= 3 && remaining[0] == "data" {
		keyword = "data"
		resType = remaining[1]
		resName = remaining[2]
	} else if len(remaining) >= 2 {
		keyword = "resource"
		resType = remaining[0]
		resName = remaining[1]
	} else {
		return "", 0
	}

	// Search for: resource "aws_instance" "example" or data "aws_ami" "ubuntu"
	pattern := keyword + ` "` + resType + `" "` + resName + `"`

	for _, f := range m.files {
		if f.IsVars {
			continue
		}
		content, err := m.runner.ReadFile(f.Path)
		if err != nil {
			continue
		}
		for lineNum, line := range strings.Split(content, "\n") {
			if strings.Contains(strings.TrimSpace(line), pattern) {
				return f.Path, lineNum + 1
			}
		}
	}

	return "", 0
}

// ─── Context Help Hints ─────────────────────────────────

// contextKeysFor returns the context-specific key hints for a panel.
func contextKeysFor(panel PanelID) []keyHint {
	switch panel {
	case PanelFiles:
		return []keyHint{{"e", "edit"}}
	case PanelResources:
		return []keyHint{
			{"e", "file"},
			{"s", "show"},
			{"t", "taint"},
			{"u", "untaint"},
			{"x", "rm"},
			{"T", "target"},
		}
	case PanelModules:
		return []keyHint{
			{"e", "edit"},
			{"o", "open dir"},
			{"T", "target"},
		}
	case PanelWorkspaces:
		return []keyHint{
			{"n", "new"},
			{"x", "delete"},
		}
	case PanelVarFiles:
		return []keyHint{{"e", "edit"}}
	}
	return nil
}

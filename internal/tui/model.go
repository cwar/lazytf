package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cwar/lazytf/internal/terraform"
	"github.com/cwar/lazytf/internal/ui"
)

// Focus tracks whether left panels or right detail pane is focused.
type Focus int

const (
	FocusLeft Focus = iota
	FocusRight
)

// Model is the main application model.
type Model struct {
	// Terraform
	runner    *terraform.Runner
	workDir   string
	version   string
	workspace string
	gitBranch string

	// Layout
	width  int
	height int
	focus  Focus

	// Left column: stacked sub-panels (lazygit style)
	panels      []*SubPanel
	activePanel PanelID

	// Right detail pane
	detailTitle      string
	detailLines      []string // raw lines
	highlightedLines []string // pre-rendered with ANSI
	isHighlighted    bool
	detailScroll     int

	// Data
	files    []terraform.TfFile
	fileTree *terraform.DirTree
	resources  []terraform.Resource
	modules    []terraform.ModuleCall
	workspaces *terraform.WorkspaceInfo
	outputs    []terraform.Output
	graph      *terraform.Graph

	// Active selection
	selectedVarFile string // manually chosen var file (empty = use auto)
	varFileManual   bool   // true if user explicitly chose a var file

	// Status / loading
	statusMsg  string
	isLoading  bool
	loadingMsg string
	spinner    spinner.Model

	// Streaming output control
	followOutput     bool              // auto-scroll to follow new streaming output
	planHighlighter  *ui.PlanHighlighter // stateful plan line highlighter for streaming

	// Apply/Destroy result pinning (keeps output visible after completion)
	applyResult bool // true when showing apply/destroy output; cleared on dismiss or new cmd

	// Plan review state (plan → review → apply workflow)
	pendingPlanFile string       // path to saved plan file awaiting apply
	planReview      bool         // true when showing a plan for review before apply/destroy
	planIsDestroy   bool         // true if the pending plan is a destroy plan
	planChanges     []planChange // parsed resource changes from plan output
	planChangeCur   int          // currently selected change index
	planFocusView   bool         // true = show one resource at a time
	planCompactDiff      bool     // true = collapse unchanged heredoc lines
	compactLines         []string // compacted raw lines (nil = not computed)
	compactHighlighted   []string // compacted highlighted lines

	// Last plan recall (saved plan available for re-review)
	lastPlanFile        string       // path to saved plan file from last dismissed review
	lastPlanIsDestroy   bool         // whether the saved plan is a destroy
	lastPlanLines       []string     // saved plan output lines
	lastPlanHighlighted []string     // saved highlighted plan output
	lastPlanChanges     []planChange // saved parsed resource changes
	lastPlanTitle       string       // saved detail title

	// Command log
	cmdOutput []string

	// Overlays
	showHelp    bool
	showLog     bool
	showGraph   bool
	showConfirm bool
	confirmAction string
	confirmMsg    string
	confirmData   string

	// Input overlay (for workspace name, etc.)
	showInput   bool
	inputPrompt string
	inputValue  string
	inputAction string
}

// tempPlanFile creates a unique temporary file path for saving terraform plans.
// Uses os.CreateTemp for uniqueness, then closes/returns the path so terraform
// can write to it. This avoids PID-based collisions between concurrent instances.
func tempPlanFile() string {
	f, err := os.CreateTemp("", "lazytf-*.tfplan")
	if err != nil {
		// Fallback — still better than nothing
		return filepath.Join(os.TempDir(), fmt.Sprintf("lazytf-%d.tfplan", os.Getpid()))
	}
	path := f.Name()
	f.Close()
	return path
}

// NewModel creates a new TUI model.
func NewModel(workDir string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = ui.SpinnerLabel

	runner := terraform.NewRunner(workDir)

	panels := make([]*SubPanel, PanelCount)
	for i := PanelID(0); i < PanelCount; i++ {
		panels[i] = &SubPanel{ID: i}
	}

	m := Model{
		runner:      runner,
		workDir:     workDir,
		version:     runner.Version(),
		focus:       FocusLeft,
		panels:      panels,
		activePanel: PanelFiles,
		spinner:     s,
		detailTitle: "Welcome",
		detailLines: []string{
			"",
			"  ⚡ lazytf",
			"",
			"  Loading terraform project...",
		},
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadAllData(),
	)
}

// ─── Messages ────────────────────────────────────────────

type dataLoadedMsg struct {
	files      []terraform.TfFile
	resources  []terraform.Resource
	modules    []terraform.ModuleCall
	workspaces *terraform.WorkspaceInfo
	outputs    []terraform.Output
	gitBranch  string
	errors     []string // non-fatal load errors to surface in the status bar
}

type cmdStartMsg struct {
	title string
}

// cmdStreamLineMsg delivers a single line of live output from a streaming command.
// It carries the channel and error pointer so Update can chain the next read.
type cmdStreamLineMsg struct {
	title  string
	line   string
	ch     <-chan string
	cmdErr *error
}

type cmdDoneMsg struct {
	title    string
	output   string
	err      error
	streamed bool // true if output was already streamed line-by-line
}

type stateShowMsg struct {
	address string
	output  string
}

type graphLoadedMsg struct {
	graph *terraform.Graph
	raw   string
}

// ─── Commands ────────────────────────────────────────────

func (m *Model) loadAllData() tea.Cmd {
	return func() tea.Msg {
		var errs []string

		files, err := m.runner.ListFiles()
		if err != nil {
			errs = append(errs, "files: "+err.Error())
		}
		resources, err := m.runner.StateList()
		if err != nil {
			errs = append(errs, "state: "+err.Error())
		}
		modules, err := m.runner.ParseModules()
		if err != nil {
			errs = append(errs, "modules: "+err.Error())
		}
		workspaces, err := m.runner.Workspaces()
		if err != nil {
			errs = append(errs, "workspaces: "+err.Error())
		}
		outputs, err := m.runner.Outputs()
		if err != nil {
			errs = append(errs, "outputs: "+err.Error())
		}
		gitBranch := detectGitBranch(m.workDir)

		return dataLoadedMsg{
			files:      files,
			resources:  resources,
			modules:    modules,
			workspaces: workspaces,
			outputs:    outputs,
			gitBranch:  gitBranch,
			errors:     errs,
		}
	}
}

func (m *Model) loadStateShow(address string) tea.Cmd {
	return func() tea.Msg {
		out, _ := m.runner.StateShow(address)
		return stateShowMsg{address: address, output: out}
	}
}

func (m *Model) loadGraph() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.runner.Graph()
		if err != nil {
			return graphLoadedMsg{raw: "Error: " + raw}
		}
		g := terraform.ParseDOT(raw)
		return graphLoadedMsg{graph: g, raw: raw}
	}
}

func (m *Model) runTfCmd(title string, fn func() (string, error)) tea.Cmd {
	return tea.Sequence(
		func() tea.Msg { return cmdStartMsg{title: title} },
		func() tea.Msg {
			out, err := fn()
			return cmdDoneMsg{title: title, output: out, err: err}
		},
	)
}

func (m *Model) runTfCmdStream(title string, fn func(onLine func(string)) error) tea.Cmd {
	ch := make(chan string, 64)
	var cmdErr error

	go func() {
		cmdErr = fn(func(line string) {
			ch <- line
		})
		close(ch)
	}()

	return tea.Sequence(
		func() tea.Msg { return cmdStartMsg{title: title} },
		readStreamLine(title, ch, &cmdErr),
	)
}

// readStreamLine returns a tea.Cmd that reads the next line from a streaming
// command's channel. When the channel closes (command finished), it returns
// cmdDoneMsg instead.
func readStreamLine(title string, ch <-chan string, cmdErr *error) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return cmdDoneMsg{title: title, err: *cmdErr, streamed: true}
		}
		return cmdStreamLineMsg{title: title, line: line, ch: ch, cmdErr: cmdErr}
	}
}

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
		m.focus = FocusRight // auto-focus detail pane to allow scrolling
		return m, nil

	case cmdStreamLineMsg:
		m.detailLines = append(m.detailLines, msg.line)
		m.highlightedLines = append(m.highlightedLines, m.planHighlighter.HighlightLine(msg.line))
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
		return m, readStreamLine(msg.title, msg.ch, msg.cmdErr)

	case dataLoadedMsg:
		m.files = msg.files
		m.resources = msg.resources
		m.modules = msg.modules
		m.workspaces = msg.workspaces
		m.outputs = msg.outputs
		m.gitBranch = msg.gitBranch
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

	case clipboardMsg:
		if msg.err != nil {
			m.statusMsg = ui.ErrorStyle.Render("Clipboard: " + msg.err.Error())
		} else {
			addr := ""
			if len(m.planChanges) > 0 {
				addr = " — " + m.planChanges[m.planChangeCur].Address
			}
			m.statusMsg = ui.SuccessStyle.Render("✓ Copied to clipboard" + addr)
		}
		return m, nil

	case cmdDoneMsg:
		m.isLoading = false
		m.loadingMsg = ""
		m.followOutput = false

		if msg.err != nil {
			m.statusMsg = ui.ErrorStyle.Render("✗ " + msg.title + " failed")
		} else {
			m.statusMsg = ui.SuccessStyle.Render("✓ " + msg.title + " complete")
		}
		m.detailTitle = msg.title

		if msg.streamed {
			// Output was already streamed line-by-line into detailLines.
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
				// Plan failed — clean up
				os.Remove(m.pendingPlanFile)
				m.pendingPlanFile = ""
				m.planIsDestroy = false
			} else {
				// Plan succeeded — enter review mode
				m.planReview = true
				m.planChanges = parsePlanChanges(m.detailLines)
				m.planChangeCur = 0
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
			m.autoSelectVarFile()
		}

		// Reload data
		return m, m.loadAllData()
	}

	return m, nil
}

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
			return m, m.runTfCmdStream(title, func(onLine func(string)) error {
				defer os.Remove(planFile) // clean up plan file after apply
				return m.runner.ApplyPlanStream(planFile, onLine)
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
	if m.isLoading && isOperationKey(key) {
		m.statusMsg = busyMsg()
		return m, nil
	}
	switch key {
	case "p":
		return m, m.runTfCmdStream("Plan", func(onLine func(string)) error {
			return m.runner.PlanStream(m.selectedVarFile, onLine)
		})
	case "a":
		m.clearLastPlan() // new plan replaces any saved plan
		planFile := tempPlanFile()
		m.pendingPlanFile = planFile
		m.planIsDestroy = false
		varFile := m.selectedVarFile
		return m, m.runTfCmdStream("Plan → Apply", func(onLine func(string)) error {
			return m.runner.PlanSaveStream(varFile, planFile, false, onLine)
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
		m.clearLastPlan() // new plan replaces any saved plan
		planFile := tempPlanFile()
		m.pendingPlanFile = planFile
		m.planIsDestroy = true
		varFile := m.selectedVarFile
		return m, m.runTfCmdStream("Plan → Destroy", func(onLine func(string)) error {
			return m.runner.PlanSaveStream(varFile, planFile, true, onLine)
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

	// Commands also work from right pane
	case "p", "a", "i", "v":
		if m.isLoading {
			m.statusMsg = busyMsg()
			return m, nil
		}
		switch key {
		case "p":
			return m, m.runTfCmdStream("Plan", func(onLine func(string)) error {
				return m.runner.PlanStream(m.selectedVarFile, onLine)
			})
		case "a":
			m.clearLastPlan() // new plan replaces any saved plan
			planFile := tempPlanFile()
			m.pendingPlanFile = planFile
			m.planIsDestroy = false
			varFile := m.selectedVarFile
			return m, m.runTfCmdStream("Plan → Apply", func(onLine func(string)) error {
				return m.runner.PlanSaveStream(varFile, planFile, false, onLine)
			})
		case "i":
			return m, m.runTfCmd("Init", func() (string, error) {
				return m.runner.Init()
			})
		case "v":
			return m, m.runTfCmd("Validate", func() (string, error) {
				return m.runner.Validate()
			})
		}
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
// single-resource views. This is safe to call from View() (value receiver)
// as it reads but does not write model state.
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

// ─── Selection Changed ───────────────────────────────────

// onSelectionChanged updates the detail pane based on current selection.
func (m *Model) onSelectionChanged() {
	panel := m.panels[m.activePanel]
	item := panel.SelectedItem()

	switch m.activePanel {
	case PanelStatus:
		m.showStatusDetail()

	case PanelFiles:
		if item == nil {
			m.detailTitle = "Files"
			m.setDetailPlain("No files found")
			return
		}
		if f, ok := item.Data.(terraform.TfFile); ok {
			content, err := m.runner.ReadFile(f.Path)
			if err != nil {
				m.detailTitle = f.RelPath
				m.setDetailPlain("Error: " + err.Error())
			} else {
				m.detailTitle = f.RelPath
				m.setDetailContent(content, true)
			}
		}

	case PanelResources:
		if item == nil {
			m.detailTitle = "Resources"
			m.setDetailPlain("No resources in state")
			return
		}
		if r, ok := item.Data.(terraform.Resource); ok {
			m.detailTitle = "Loading: " + r.Address
			m.setDetailPlain("Loading state for " + r.Address + "...")
			// This returns a proper tea.Cmd instead of a raw goroutine!
			// The stateShowMsg will update the detail pane when it arrives.
		}

	case PanelModules:
		if item == nil {
			m.detailTitle = "Modules"
			m.setDetailPlain("No modules found")
			return
		}
		if mod, ok := item.Data.(terraform.ModuleCall); ok {
			m.showModuleDetail(mod)
		}

	case PanelWorkspaces:
		if item == nil {
			return
		}
		wsName := item.Label
		m.detailTitle = "Workspace: " + wsName
		info := fmt.Sprintf("  Workspace: %s\n", wsName)
		if wsName == m.workspace {
			info += "\n  ✓ This is the current workspace\n"
		} else {
			info += "\n  Press Enter to switch to this workspace\n"
		}
		m.setDetailPlain(info)

	case PanelVarFiles:
		if item == nil {
			return
		}
		if f, ok := item.Data.(terraform.TfFile); ok {
			content, err := m.runner.ReadFile(f.Path)
			if err != nil {
				m.detailTitle = f.RelPath
				m.setDetailPlain("Error: " + err.Error())
			} else {
				m.detailTitle = f.RelPath
				m.setDetailContent(content, true)
			}
		}
	}
}

// onSelectionChangedCmd returns any async commands needed after selection change.
func (m *Model) onSelectionChangedCmd() tea.Cmd {
	if m.activePanel == PanelResources {
		panel := m.panels[PanelResources]
		if item := panel.SelectedItem(); item != nil {
			if r, ok := item.Data.(terraform.Resource); ok {
				return m.loadStateShow(r.Address)
			}
		}
	}
	return nil
}

func (m *Model) showStatusDetail() {
	m.detailTitle = "Status"

	init := "✗ Not initialized"
	if m.runner.IsInitialized() {
		init = "✓ Initialized"
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  ⚡ lazytf — "+m.version)
	lines = append(lines, "")
	lines = append(lines, "  Directory:  "+m.workDir)
	lines = append(lines, "  Workspace:  "+m.workspace)
	lines = append(lines, "  State:      "+init)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Files:      %d (.tf) + %d (.tfvars)", countNonVars(m.files), countVars(m.files)))
	lines = append(lines, fmt.Sprintf("  Resources:  %d in state", len(m.resources)))
	lines = append(lines, fmt.Sprintf("  Modules:    %d module calls", len(m.modules)))
	if m.outputs != nil {
		lines = append(lines, fmt.Sprintf("  Outputs:    %d", len(m.outputs)))
	}
	if m.selectedVarFile != "" {
		lines = append(lines, "")
		lines = append(lines, "  Active var-file: "+shortPath(m.selectedVarFile))
	}
	lines = append(lines, "")
	lines = append(lines, "  Press ? for keyboard shortcuts")

	m.setDetailPlain(strings.Join(lines, "\n"))
}

func (m *Model) showModuleDetail(mod terraform.ModuleCall) {
	m.detailTitle = "Module: " + mod.Name

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  📦 Module: "+mod.Name)
	lines = append(lines, "")
	lines = append(lines, "  Source:      "+mod.Source)
	lines = append(lines, "  Display:     "+mod.ModuleSourceDisplay())
	lines = append(lines, "  Defined in:  "+mod.SourceFile)
	if mod.Version != "" {
		lines = append(lines, "  Version:     "+mod.Version)
	}
	localDir := mod.ModuleDir(m.workDir)
	if localDir != "" {
		lines = append(lines, "  Local path:  "+localDir)
	}
	if len(mod.Variables) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  Input variables passed:")
		for _, v := range mod.Variables {
			lines = append(lines, "    • "+v)
		}
	}

	// If local, show the files in that directory
	if localDir != "" {
		lines = append(lines, "")
		lines = append(lines, "  Files in module:")
		for _, f := range m.files {
			if strings.HasPrefix(f.RelPath, localDir+"/") || f.Dir == localDir {
				lines = append(lines, "    📄 "+f.RelPath)
			}
		}
	}

	m.setDetailPlain(strings.Join(lines, "\n"))
}

func (m *Model) renderGraphDetail() {
	if m.graph == nil {
		m.detailTitle = "Graph"
		m.setDetailPlain("Loading graph...")
		return
	}

	m.detailTitle = "Dependency Graph"
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  " + m.graph.Summary() + "\n")
	sb.WriteString("\n")
	sb.WriteString("  ── Tree View ──────────────────────────────\n")
	sb.WriteString("\n")

	tree := m.graph.RenderTree()
	for _, line := range strings.Split(tree, "\n") {
		sb.WriteString("  " + line + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString("  ── Dependency Details ─────────────────────\n")
	sb.WriteString("\n")

	details := m.graph.RenderASCII()
	for _, line := range strings.Split(details, "\n") {
		sb.WriteString("  " + line + "\n")
	}

	m.setDetailPlain(sb.String())
}

// ─── Detail Pane Helpers ─────────────────────────────────

func (m *Model) setDetailPlain(content string) {
	m.detailLines = strings.Split(content, "\n")
	m.highlightedLines = nil
	m.isHighlighted = false
	m.detailScroll = 0
}

func (m *Model) setDetailContent(content string, tryHighlight bool) {
	m.detailLines = strings.Split(content, "\n")
	m.detailScroll = 0

	if tryHighlight {
		hl := ui.HighlightTfContent(content, m.detailTitle)
		if hl != nil {
			m.highlightedLines = hl
			m.isHighlighted = true
			return
		}
		// Try highlighting as HCL anyway (for state show output)
		if strings.Contains(content, " = ") || strings.Contains(content, " {") {
			hl = ui.HighlightHCL(content, true)
			m.highlightedLines = hl
			m.isHighlighted = true
			return
		}
	}
	m.highlightedLines = nil
	m.isHighlighted = false
}

// ─── Panel Data ──────────────────────────────────────────

func (m *Model) rebuildAllPanels() {
	m.rebuildStatusPanel()
	m.rebuildFilesPanel()
	m.rebuildResourcesPanel()
	m.rebuildModulesPanel()
	m.rebuildWorkspacesPanel()
	m.rebuildVarFilesPanel()
}

func (m *Model) rebuildStatusPanel() {
	p := m.panels[PanelStatus]
	p.Items = []PanelItem{}

	ws := m.workspace
	if ws == "" {
		ws = "default"
	}
	init := "✗ not init"
	if m.runner.IsInitialized() {
		init = "✓ initialized"
	}
	dir := shortPath(m.workDir)

	p.Items = append(p.Items, PanelItem{
		Label: fmt.Sprintf("%s → %s  %s", dir, ws, init),
		Icon:  "✓",
	})
}

func (m *Model) rebuildFilesPanel() {
	p := m.panels[PanelFiles]
	oldCursor := p.Cursor
	p.Items = nil

	lastDir := ""
	for _, f := range m.files {
		if f.IsVars {
			continue
		}
		// Insert directory headers
		if f.Dir != "" && f.Dir != lastDir {
			p.Items = append(p.Items, PanelItem{
				Label: f.Dir + "/",
				Icon:  "📁",
				Data:  f, // clicking shows first file
			})
			lastDir = f.Dir
		}
		indent := ""
		if f.Dir != "" {
			indent = "  "
		}
		p.Items = append(p.Items, PanelItem{
			Label: indent + f.Name,
			Data:  f,
		})
		if f.Dir != lastDir && f.Dir == "" {
			lastDir = ""
		}
	}

	if oldCursor < len(p.Items) {
		p.Cursor = oldCursor
	} else if len(p.Items) > 0 {
		p.Cursor = 0
	}
}

func (m *Model) rebuildResourcesPanel() {
	p := m.panels[PanelResources]
	oldCursor := p.Cursor
	p.Items = nil

	for _, r := range m.resources {
		label := ""
		if r.Module != "" {
			label = r.Module + "."
		}
		label += r.Type + "." + r.Name
		p.Items = append(p.Items, PanelItem{
			Label: label,
			Icon:  "◆",
			Data:  r,
		})
	}

	if oldCursor < len(p.Items) {
		p.Cursor = oldCursor
	} else if len(p.Items) > 0 {
		p.Cursor = 0
	}
}

func (m *Model) rebuildModulesPanel() {
	p := m.panels[PanelModules]
	oldCursor := p.Cursor
	p.Items = nil

	for _, mod := range m.modules {
		display := mod.ModuleSourceDisplay()
		label := mod.Name
		if display != "" {
			label += " (" + display + ")"
		}
		p.Items = append(p.Items, PanelItem{
			Label: label,
			Icon:  "📦",
			Data:  mod,
		})
	}

	if oldCursor < len(p.Items) {
		p.Cursor = oldCursor
	} else if len(p.Items) > 0 {
		p.Cursor = 0
	}
}

func (m *Model) rebuildWorkspacesPanel() {
	p := m.panels[PanelWorkspaces]
	oldCursor := p.Cursor
	p.Items = nil

	if m.workspaces != nil {
		for _, ws := range m.workspaces.Workspaces {
			icon := " "
			if ws == m.workspace {
				icon = "●"
			}
			p.Items = append(p.Items, PanelItem{
				Label: ws,
				Icon:  icon,
			})
		}
	}

	if oldCursor < len(p.Items) {
		p.Cursor = oldCursor
	} else if len(p.Items) > 0 {
		p.Cursor = 0
	}
}

// autoSelectVarFile finds the best matching .tfvars file for the current
// workspace. It matches workspace name against the filename stem, e.g.
// workspace "dev-gew4" matches "dev-gew4.tfvars" or "envs/dev-gew4.tfvars".
// If the user has manually selected a var file, this is a no-op.
func (m *Model) autoSelectVarFile() {
	if m.varFileManual {
		return
	}
	m.selectedVarFile = m.matchVarFileForWorkspace(m.workspace)
}

// matchVarFileForWorkspace returns the path of the best matching .tfvars file
// for the given workspace name, or "" if no match is found.
//
// Matching priority:
//  1. Exact stem match in root dir:  "dev-gew4.tfvars"
//  2. Exact stem match with .auto:   "dev-gew4.auto.tfvars"
//  3. Exact stem match in subdirs:   "envs/dev-gew4.tfvars"
func (m *Model) matchVarFileForWorkspace(ws string) string {
	if ws == "" || ws == "default" {
		return ""
	}

	var rootMatch, autoMatch, subDirMatch string

	for _, f := range m.files {
		if !f.IsVars {
			continue
		}
		stem := strings.TrimSuffix(f.Name, ".tfvars")
		stem = strings.TrimSuffix(stem, ".auto") // handle .auto.tfvars

		if stem != ws {
			continue
		}

		isAuto := strings.HasSuffix(f.Name, ".auto.tfvars")

		switch {
		case f.Dir == "" && !isAuto:
			rootMatch = f.Path
		case f.Dir == "" && isAuto:
			autoMatch = f.Path
		case subDirMatch == "":
			subDirMatch = f.Path
		}
	}

	// Priority: root exact > auto > subdir
	switch {
	case rootMatch != "":
		return rootMatch
	case autoMatch != "":
		return autoMatch
	default:
		return subDirMatch
	}
}

func (m *Model) rebuildVarFilesPanel() {
	p := m.panels[PanelVarFiles]
	oldCursor := p.Cursor
	p.Items = nil

	for _, f := range m.files {
		if !f.IsVars {
			continue
		}
		icon := " "
		if f.Path == m.selectedVarFile {
			if m.varFileManual {
				icon = "●" // manually selected
			} else {
				icon = "◉" // auto-matched from workspace
			}
		}
		label := f.Name
		if f.Dir != "" {
			label = f.RelPath
		}
		p.Items = append(p.Items, PanelItem{
			Label: label,
			Icon:  icon,
			Data:  f,
		})
	}

	if oldCursor < len(p.Items) {
		p.Cursor = oldCursor
	} else if len(p.Items) > 0 {
		p.Cursor = 0
	}
}

// ─── View ────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Overlays
	if m.showHelp {
		return m.renderHelp()
	}
	if m.showConfirm {
		return m.renderConfirm()
	}
	if m.showInput {
		return m.renderInput()
	}

	// Layout widths
	leftWidth := m.width * 30 / 100
	if leftWidth < 32 {
		leftWidth = 32
	}
	if leftWidth > 55 {
		leftWidth = 55
	}
	rightWidth := m.width - leftWidth - 1 // 1 for separator

	// Heights
	statusBarH := 1
	helpHintH := 1
	contentH := m.height - statusBarH - helpHintH

	// Allocate panel heights
	AllocatePanelHeights(m.panels[:], contentH)

	// Render left column
	left := m.renderLeftColumn(leftWidth, contentH)

	// Render right column
	right := m.renderDetailPane(rightWidth, contentH)

	// Separator
	sep := lipgloss.NewStyle().
		Foreground(ui.DimGray).
		Render(strings.Repeat("│\n", contentH))

	// Compose main content
	content := lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right)

	// Status bar
	statusBar := m.renderStatusBar()

	// Help hint
	helpHint := m.renderHelpHint()

	return lipgloss.JoinVertical(lipgloss.Left,
		content,
		statusBar,
		helpHint,
	)
}

func (m Model) renderLeftColumn(width, height int) string {
	var parts []string
	totalH := 0

	for _, p := range m.panels {
		isActive := p.ID == m.activePanel
		rendered := p.Render(width, isActive, m.focus == FocusLeft)
		parts = append(parts, rendered)
		totalH += p.Height
	}

	col := strings.Join(parts, "\n")

	// If the column is shorter than available height, pad
	lines := strings.Split(col, "\n")
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines[:height], "\n")
}

func (m Model) renderDetailPane(width, height int) string {
	isActive := m.focus == FocusRight

	// Title
	var titleStyle lipgloss.Style
	if isActive {
		titleStyle = ui.PanelTitle
	} else {
		titleStyle = ui.InactivePanelTitle
	}

	detailTitle := m.detailTitle
	if m.planReview && len(m.planChanges) > 0 {
		c := m.planChanges[m.planChangeCur]
		modeTag := ""
		if m.planFocusView {
			modeTag = "🔍 "
		}
		detailTitle = fmt.Sprintf("%s[%d/%d] %s%s %s",
			modeTag, m.planChangeCur+1, len(m.planChanges),
			actionIcon(c.Action), actionLabel(c.Action), c.Address)
	}
	title := titleStyle.Render(" " + detailTitle + " ")

	// Content lines
	useHL := m.isHighlighted && len(m.highlightedLines) == len(m.detailLines)

	// Handle log/graph overlay in right pane
	var sourceLines []string
	var hlLines []string
	useOverlayHL := false

	if m.showLog {
		sourceLines = m.cmdOutput
		if m.isLoading {
			sourceLines = append(append([]string{}, sourceLines...), "", "  ⟳ "+m.loadingMsg)
		}
		if len(sourceLines) == 0 {
			sourceLines = []string{"", "  No commands run yet", "", "  Run a terraform command (p/a/i/v) to see output here"}
		}
	} else if m.showGraph {
		sourceLines = m.detailLines
	} else if m.planReview {
		// Plan review: use planViewLines which handles focus + compact
		sourceLines, hlLines = m.planViewLines()
		useOverlayHL = len(hlLines) == len(sourceLines) && len(hlLines) > 0
	} else {
		sourceLines = m.detailLines
		hlLines = m.highlightedLines
		useOverlayHL = useHL
	}

	// Scroll indicator
	scrollInfo := ""
	visH := height - 2 // title + bottom
	if visH < 1 {
		visH = 1
	}
	totalLines := len(sourceLines)
	if totalLines > visH {
		pct := 0
		denom := totalLines - visH
		if denom > 0 {
			pct = m.detailScroll * 100 / denom
		}
		scrollInfo = ui.DimItem.Render(fmt.Sprintf(" %d%% ", pct))
	}

	titlePad := width - lipgloss.Width(title) - lipgloss.Width(scrollInfo)
	if titlePad < 0 {
		titlePad = 0
	}
	titleLine := title + ui.DimItem.Render(strings.Repeat("─", titlePad)) + scrollInfo

	// Override title for overlays
	if m.showLog {
		titleLine = ui.PanelTitle.Render(" Command Log ") + ui.DimItem.Render(strings.Repeat("─", width-15))
	} else if m.showGraph {
		titleLine = ui.PanelTitle.Render(" Dependency Graph ") + ui.DimItem.Render(strings.Repeat("─", width-20))
		if width-20 < 0 {
			titleLine = ui.PanelTitle.Render(" Graph ")
		}
	}

	var lines []string
	for i := m.detailScroll; i < len(sourceLines) && len(lines) < visH; i++ {
		var line string
		if useOverlayHL && i < len(hlLines) {
			line = hlLines[i]
		} else {
			line = sourceLines[i]
		}
		// Soft-wrap long lines to fit the pane width
		wrapped := ui.WrapPlanLines([]string{line}, width)
		for _, wl := range wrapped {
			if len(lines) >= visH {
				break
			}
			lines = append(lines, wl)
		}
	}

	// Pad
	for len(lines) < visH {
		lines = append(lines, "")
	}

	return titleLine + "\n" + strings.Join(lines, "\n")
}

func (m Model) renderStatusBar() string {
	branch := ""
	if m.gitBranch != "" {
		branch = ui.GitBranchIcon.Render(" ") + " " + ui.GitBranchName.Render(m.gitBranch) + "  "
	}

	ws := ui.StatusKey.Render("workspace:") + " " + ui.StatusValue.Render(m.workspace)

	varFile := ""
	if m.selectedVarFile != "" {
		varLabel := "var-file:"
		if !m.varFileManual {
			varLabel = "var-file(auto):"
		}
		varFile = "  " + ui.StatusKey.Render(varLabel) + " " + ui.StatusValue.Render(shortPath(m.selectedVarFile))
	}

	status := m.statusMsg
	if m.isLoading {
		status = m.spinner.View() + " " + m.loadingMsg
	}

	left := branch + ws + varFile
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(status) - 2
	if pad < 1 {
		pad = 1
	}

	return ui.StatusBar.Width(m.width).Render(left + strings.Repeat(" ", pad) + status)
}

func (m Model) renderHelpHint() string {
	if m.applyResult {
		title := m.detailTitle
		hint := ui.SuccessStyle.Render("▶ "+title+" result") + "  " +
			ui.HelpKey.Render("esc") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("dismiss") + "  " +
			ui.HelpKey.Render("j/k") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("scroll") + "  " +
			ui.HelpKey.Render("d/u") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("page dn/up") + "  " +
			ui.HelpKey.Render("g/G") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("top/bottom")
		return hint
	}

	if m.planReview {
		// Show prominent review-mode actions
		action := "apply"
		if m.planIsDestroy {
			action = "DESTROY"
		}
		focusLabel := "focus"
		if m.planFocusView {
			focusLabel = "full plan"
		}
		compactLabel := "compact"
		if m.planCompactDiff {
			compactLabel = "full diff"
		}
		hint := ui.WarningStyle.Render("▶ Review plan") + "  " +
			ui.HelpKey.Render("y") + ui.HelpSep.Render(":") + ui.SuccessStyle.Render(action) + "  " +
			ui.HelpKey.Render("esc") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("cancel") + "  " +
			ui.HelpKey.Render("n/N") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("next/prev") + "  " +
			ui.HelpKey.Render("enter") + ui.HelpSep.Render(":") + ui.HelpDesc.Render(focusLabel) + "  " +
			ui.HelpKey.Render("z") + ui.HelpSep.Render(":") + ui.HelpDesc.Render(compactLabel) + "  "
		if m.planFocusView {
			hint += ui.HelpKey.Render("j/k") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("scroll") + "  "
		}
		hint += ui.HelpKey.Render("d/u") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("page dn/up") + "  " +
			ui.HelpKey.Render("c") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("copy")
		if len(m.planChanges) > 0 {
			hint += "  " + ui.DimItem.Render(fmt.Sprintf("(%d resources)", len(m.planChanges)))
		}
		return hint
	}

	// Context-specific keys for the active panel
	ctxKeys := contextKeysFor(m.activePanel)

	// Build context section (panel-specific keys, shown first)
	var ctxSection string
	if len(ctxKeys) > 0 {
		panelName := panelDefs[m.activePanel].Label
		ctxParts := []string{ui.StatusKey.Render("[" + panelName + "]")}
		for _, k := range ctxKeys {
			ctxParts = append(ctxParts,
				ui.HelpKey.Render(k.Key)+ui.HelpSep.Render(":")+ui.HelpDesc.Render(k.Desc))
		}
		ctxSection = strings.Join(ctxParts, " ") + " " + ui.DimItem.Render("│") + " "
	}

	// Global keys — show more when no context keys eat up space
	var globalKeys []struct{ key, desc string }
	if len(ctxKeys) == 0 {
		globalKeys = []struct{ key, desc string }{
			{"p", "plan"},
			{"a", "apply"},
			{"i", "init"},
			{"v", "validate"},
			{"D", "destroy"},
			{"G", "graph"},
			{"f", "fmt"},
			{"l", "log"},
			{"r", "refresh"},
			{"1-6", "panels"},
			{"tab", "switch"},
			{"?", "help"},
			{"q", "quit"},
		}
	} else {
		globalKeys = []struct{ key, desc string }{
			{"p", "plan"},
			{"a", "apply"},
			{"i", "init"},
			{"D", "destroy"},
			{"r", "refresh"},
			{"?", "help"},
			{"q", "quit"},
		}
	}

	// Show recall hint if a saved plan is available
	if m.hasLastPlan() {
		globalKeys = append([]struct{ key, desc string }{{"R", "recall plan"}}, globalKeys...)
	}

	var globalParts []string
	for _, k := range globalKeys {
		globalParts = append(globalParts,
			ui.HelpKey.Render(k.key)+ui.HelpSep.Render(":")+ui.HelpDesc.Render(k.desc))
	}

	globalSection := strings.Join(globalParts, " ")
	line := ctxSection + globalSection

	// Truncate global keys from the right if too wide
	for lipgloss.Width(line) > m.width && len(globalParts) > 2 {
		globalParts = globalParts[:len(globalParts)-1]
		globalSection = strings.Join(globalParts, " ")
		line = ctxSection + globalSection
	}

	return line
}

func (m Model) renderHelp() string {
	title := ui.Logo.Render("⚡ lazytf — Keyboard Shortcuts")

	sections := []struct {
		name string
		keys []struct{ key, desc string }
	}{
		{"Navigation", []struct{ key, desc string }{
			{"j/k ↑/↓", "Move up/down in active panel"},
			{"1-6", "Jump to panel by number"},
			{"[ ]", "Cycle through panels"},
			{"Tab", "Switch between left panels / right detail"},
			{"Enter", "Select (switch workspace, toggle varfile, focus detail)"},
			{"g/G", "Top/bottom of detail pane"},
			{"d/u", "Page down/up in detail pane"},
		}},
		{"Terraform Commands", []struct{ key, desc string }{
			{"p", "terraform plan"},
			{"a", "terraform apply (plan → review → apply)"},
			{"i", "terraform init"},
			{"v", "terraform validate"},
			{"f/F", "terraform fmt check / fix"},
			{"D", "terraform destroy (plan → review → destroy)"},
			{"R", "Recall last plan (re-enter review)"},
			{"P", "Show providers"},
		}},
		{"Plan Review", []struct{ key, desc string }{
			{"y", "Confirm apply/destroy"},
			{"esc/q", "Dismiss (saves plan for recall)"},
			{"n/N", "Next/previous resource change"},
			{"j/k ↑/↓", "Scroll within resource (focus mode)"},
			{"d/u", "Page down/up"},
			{"g/G", "Top/bottom"},
			{"enter", "Toggle focus mode (single resource)"},
			{"z", "Toggle compact diff (collapse unchanged heredocs)"},
			{"c", "Copy current resource diff to clipboard"},
		}},
		{"Apply/Destroy Result", []struct{ key, desc string }{
			{"esc/q", "Dismiss result and return to normal view"},
			{"j/k ↑/↓", "Scroll output"},
			{"d/u", "Page down/up"},
			{"g/G", "Top/bottom"},
		}},
		{"Views", []struct{ key, desc string }{
			{"G", "Dependency graph (from left pane)"},
			{"l", "Toggle command log"},
			{"r", "Refresh all data"},
		}},
		{"📄 Files", []struct{ key, desc string }{
			{"e", "Edit file in $EDITOR"},
		}},
		{"🏗  Resources", []struct{ key, desc string }{
			{"e", "Jump to declaring .tf file"},
			{"s", "Refresh state show"},
			{"t", "Taint resource"},
			{"u", "Untaint resource"},
			{"x", "Remove from state"},
			{"T", "Targeted plan → apply"},
		}},
		{"📦 Modules", []struct{ key, desc string }{
			{"e", "Edit module source file"},
			{"o", "Open module directory"},
			{"T", "Targeted plan → apply"},
		}},
		{"📁 Workspaces", []struct{ key, desc string }{
			{"n", "Create new workspace"},
			{"x", "Delete workspace"},
		}},
		{"⚙  Var Files", []struct{ key, desc string }{
			{"e", "Edit var file in $EDITOR"},
		}},
		{"General", []struct{ key, desc string }{
			{"?", "Toggle this help"},
			{"q/Ctrl+C", "Quit"},
		}},
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	for _, s := range sections {
		lines = append(lines, ui.SectionHeader.Render("  "+s.name))
		for _, k := range s.keys {
			key := ui.HelpKey.Width(14).Render("  " + k.key)
			desc := ui.HelpDesc.Render(k.desc)
			lines = append(lines, key+desc)
		}
		lines = append(lines, "")
	}
	lines = append(lines, ui.DimItem.Render("  Press ? or Esc to close"))

	content := strings.Join(lines, "\n")
	w := 58
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

func (m Model) renderConfirm() string {
	title := ui.WarningStyle.Render("⚠ Confirm Action")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	for _, l := range strings.Split(m.confirmMsg, "\n") {
		lines = append(lines, "  "+l)
	}
	lines = append(lines, "")
	lines = append(lines, "  "+ui.HelpKey.Render("y")+" confirm    "+ui.HelpKey.Render("n")+" cancel")

	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Yellow).
			Padding(1, 2).
			Width(50).
			Render(content),
	)
}

// ─── Helpers ─────────────────────────────────────────────

func shortPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func countNonVars(files []terraform.TfFile) int {
	n := 0
	for _, f := range files {
		if !f.IsVars {
			n++
		}
	}
	return n
}

func countVars(files []terraform.TfFile) int {
	n := 0
	for _, f := range files {
		if f.IsVars {
			n++
		}
	}
	return n
}

package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
	cancelCmd  context.CancelFunc // cancels the currently running terraform command

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

func (m *Model) runTfCmdStream(title string, fn func(ctx context.Context, onLine func(string)) error) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelCmd = cancel

	ch := make(chan string, 64)
	var cmdErr error

	go func() {
		cmdErr = fn(ctx, func(line string) {
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
				Data:  f,
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

func (m *Model) autoSelectVarFile() {
	if m.varFileManual {
		return
	}
	m.selectedVarFile = m.matchVarFileForWorkspace(m.workspace)
}

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
		stem = strings.TrimSuffix(stem, ".auto")

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
				icon = "●"
			} else {
				icon = "◉"
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

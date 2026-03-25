package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cwar/lazytf/internal/ui"
)

// ─── Multi-Workspace State ───────────────────────────────

// multiWSStatus tracks the lifecycle of a single workspace operation.
type multiWSStatus int

const (
	mwsQueued   multiWSStatus = iota
	mwsPlanning               // plan in progress
	mwsPlanned                // plan completed — has output and plan file
	mwsPlanFail               // plan failed
	mwsApplying               // apply in progress
	mwsApplied                // apply succeeded
	mwsApplyFail              // apply failed
	mwsNoChanges              // plan showed no changes
)

// multiWSItem holds the state and output for one workspace in a batch operation.
type multiWSItem struct {
	workspace   string
	varFile     string
	planFile    string
	status      multiWSStatus
	output      []string // raw plan/apply output lines
	hlOutput    []string // highlighted output lines
	changes     []planChange
	summary     string // one-line summary like "3 to add, 1 to change"
	err         error
	applyQueued bool // marked for sequential "apply all"
}

// multiWSState holds all state for the multi-workspace overlay mode.
type multiWSState struct {
	active bool   // true when multi-ws overlay is visible
	filter string // filter string used for this batch

	items  []multiWSItem
	cursor int // selected item in the workspace list
	scroll int // scroll offset for the detail output

	// Shared plan viewing state (focus, compact diff, resource navigation).
	// Data comes from the selected multiWSItem; view state is shared across items.
	view planViewer

	phase string // "planning", "reviewing", "applying", "done"

	cancel      context.CancelFunc // cancel all in-flight plans
	applyCancel context.CancelFunc // cancel the current in-flight apply

	// Streaming apply state
	followApply      bool              // auto-scroll detail pane during apply
	applyHighlighter *ui.PlanHighlighter // line-by-line highlighting for apply output
}

// multiWSConcurrency is the max number of parallel terraform operations.
const multiWSConcurrency = 4

// ─── Messages ────────────────────────────────────────────

type multiWSPlanDoneMsg struct {
	workspace string
	output    string
	err       error
}

type multiWSApplyLineMsg struct {
	workspace string
	line      string
	ch        <-chan string
	cmdErr    *error
}

type multiWSApplyDoneMsg struct {
	workspace string
	err       error
}

// ─── Entry Point ─────────────────────────────────────────

// startMultiWS sets up the multi-workspace mode and kicks off parallel plans.
func (m *Model) startMultiWS(filter string) tea.Cmd {
	if m.workspaces == nil || len(m.workspaces.Workspaces) == 0 {
		m.statusMsg = ui.ErrorStyle.Render("No workspaces available")
		return nil
	}

	// Resolve group name → filter substring
	resolved := m.config.ResolveFilter(filter)

	// Filter workspaces
	filtered := m.config.FilterWorkspaces(m.workspaces.Workspaces, resolved)
	if len(filtered) == 0 {
		m.statusMsg = ui.ErrorStyle.Render("No workspaces match filter: " + filter)
		return nil
	}

	// Build items with matched var files
	items := make([]multiWSItem, len(filtered))
	for i, ws := range filtered {
		items[i] = multiWSItem{
			workspace: ws,
			varFile:   m.matchVarFileForWorkspace(ws),
			planFile:  tempPlanFile(),
			status:    mwsPlanning, // tea.Batch starts all goroutines immediately
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	m.multiWS = multiWSState{
		active: true,
		filter: filter,
		items:  items,
		phase:  "planning",
		cancel: cancel,
	}

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

	return tea.Batch(cmds...)
}

// ─── Apply Streaming ─────────────────────────────────────

// readMultiWSApplyLine returns a tea.Cmd that reads the next line from a
// streaming multi-ws apply. When the channel closes, it returns a done msg.
func readMultiWSApplyLine(workspace string, ch <-chan string, cmdErr *error) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return multiWSApplyDoneMsg{workspace: workspace, err: *cmdErr}
		}
		return multiWSApplyLineMsg{workspace: workspace, line: line, ch: ch, cmdErr: cmdErr}
	}
}

// startMultiWSApplyStream creates a streaming apply command for one workspace.
// Output is delivered line-by-line via multiWSApplyLineMsg.
func (m *Model) startMultiWSApplyStream(workspace, planFile string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.multiWS.applyCancel = cancel

	ch := make(chan string, 64)
	var cmdErr error

	go func() {
		cmdErr = m.runner.ApplyPlanStreamWithWorkspace(ctx, workspace, planFile,
			func(line string) { ch <- line },
		)
		close(ch)
	}()

	return readMultiWSApplyLine(workspace, ch, &cmdErr)
}

// prepareItemForApply transitions a workspace item into applying state,
// moves the cursor to it, adds a separator to the output, and enables
// auto-scroll so the user can tail the apply log.
func (m *Model) prepareItemForApply(idx int) {
	item := &m.multiWS.items[idx]
	item.status = mwsApplying

	// Add separator between plan output and apply output
	sepLines := []string{"", "─── Apply Output ───", ""}
	item.output = append(item.output, sepLines...)
	h := ui.NewPlanHighlighter()
	for _, line := range sepLines {
		item.hlOutput = append(item.hlOutput, h.HighlightLine(line))
	}

	// Move cursor to this workspace and follow the apply output
	m.multiWS.cursor = idx
	m.multiWS.followApply = true
	m.multiWS.applyHighlighter = ui.NewPlanHighlighter()

	// Exit focus/compact — apply output doesn't have resource blocks
	m.multiWS.view.Reset()

	// Scroll to start of apply section
	visH := m.multiWSVisibleHeight()
	maxS := len(item.output) - visH
	if maxS < 0 {
		maxS = 0
	}
	m.multiWS.scroll = maxS
}

// handleMultiWSApplyLine processes a single streamed line from an apply.
func (m *Model) handleMultiWSApplyLine(msg multiWSApplyLineMsg) {
	for i := range m.multiWS.items {
		if m.multiWS.items[i].workspace != msg.workspace {
			continue
		}
		item := &m.multiWS.items[i]
		item.output = append(item.output, msg.line)

		// Highlight the line using the apply highlighter
		if m.multiWS.applyHighlighter != nil {
			item.hlOutput = append(item.hlOutput, m.multiWS.applyHighlighter.HighlightLine(msg.line))
		} else {
			item.hlOutput = append(item.hlOutput, msg.line)
		}

		// Auto-scroll if following and cursor is on this workspace
		if m.multiWS.followApply && m.multiWS.cursor == i {
			visH := m.multiWSVisibleHeight()
			if len(item.output) > visH {
				m.multiWS.scroll = len(item.output) - visH
			}
		}
		break
	}
}

// ─── Update Helpers ──────────────────────────────────────

// handleMultiWSPlanDone processes a completed plan for one workspace.
func (m *Model) handleMultiWSPlanDone(msg multiWSPlanDoneMsg) {
	for i := range m.multiWS.items {
		if m.multiWS.items[i].workspace != msg.workspace {
			continue
		}
		item := &m.multiWS.items[i]
		rawLines := strings.Split(msg.output, "\n")
		item.output = collapseRefreshLines(rawLines)
		fullCollapsed := strings.Join(item.output, "\n")
		item.hlOutput = ui.HighlightPlanOutput(fullCollapsed)
		item.err = msg.err

		if msg.err != nil {
			item.status = mwsPlanFail
			item.summary = "Plan failed"
			os.Remove(item.planFile)
		} else {
			item.changes = parsePlanChanges(item.output)
			item.summary = extractPlanSummary(item.output)
			if len(item.changes) == 0 && isNoChanges(item.output) {
				item.status = mwsNoChanges
				os.Remove(item.planFile)
			} else {
				item.status = mwsPlanned
			}
		}
		break
	}

	// Check if all plans are done
	allDone := true
	for _, item := range m.multiWS.items {
		if item.status == mwsPlanning {
			allDone = false
			break
		}
	}
	if allDone {
		m.multiWS.phase = "reviewing"
	}
}

// handleMultiWSApplyDone processes a completed apply for one workspace.
// Output was already streamed line-by-line via handleMultiWSApplyLine.
func (m *Model) handleMultiWSApplyDone(msg multiWSApplyDoneMsg) {
	for i := range m.multiWS.items {
		if m.multiWS.items[i].workspace != msg.workspace {
			continue
		}
		item := &m.multiWS.items[i]
		item.err = msg.err
		os.Remove(item.planFile)

		if msg.err != nil {
			item.status = mwsApplyFail
			item.summary += " → Apply failed"
		} else {
			item.status = mwsApplied
			item.summary += " → Applied ✓"
		}

		m.multiWS.followApply = false
		m.multiWS.applyHighlighter = nil
		break
	}
}

// startNextMultiWSApply finds the next workspace queued for apply, moves the
// cursor to it, and starts a streaming apply. Returns nil if all applies are done.
func (m *Model) startNextMultiWSApply() tea.Cmd {
	for i := range m.multiWS.items {
		if m.multiWS.items[i].status == mwsPlanned && m.multiWS.items[i].applyQueued {
			m.prepareItemForApply(i)
			ws := m.multiWS.items[i].workspace
			pf := m.multiWS.items[i].planFile
			return m.startMultiWSApplyStream(ws, pf)
		}
	}

	// Check if all applies are done
	anyApplying := false
	for _, item := range m.multiWS.items {
		if item.status == mwsApplying {
			anyApplying = true
			break
		}
	}
	if !anyApplying {
		m.multiWS.phase = "done"
	}
	return nil
}

// ─── Key Handling ────────────────────────────────────────

// handleMultiWSKey processes keys while in multi-workspace mode.
func (m Model) handleMultiWSKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c":
		m.closeMultiWS()
		return m, nil

	case "esc", "q":
		m.closeMultiWS()
		return m, nil

	case "f":
		// Toggle focus mode (single-resource view)
		item := m.multiWSSelectedItem()
		if item != nil && m.multiWS.view.ToggleFocus(item.changes) {
			m.multiWS.scroll = 0
			if m.multiWS.view.focusView {
				m.multiWS.view.changeCur = 0
			}
			m.multiWS.view.RecomputeCompact(item.output, item.hlOutput, item.changes)
		}
		return m, nil

	case "z":
		// Toggle compact diff mode — collapses unchanged heredoc lines
		item := m.multiWSSelectedItem()
		if item != nil {
			m.multiWS.view.compactDiff = !m.multiWS.view.compactDiff
			m.multiWS.view.RecomputeCompact(item.output, item.hlOutput, item.changes)
			m.multiWS.scroll = 0
		}
		return m, nil

	case "j", "down":
		// Scroll output down
		maxScroll := m.multiWSMaxScroll()
		if m.multiWS.scroll < maxScroll {
			m.multiWS.scroll++
		}
		return m, nil

	case "k", "up":
		// Scroll output up — disables auto-follow during apply
		if m.multiWS.scroll > 0 {
			m.multiWS.scroll--
			m.multiWS.followApply = false
		}
		return m, nil

	case "n":
		if m.multiWS.view.focusView {
			// Focus mode: next resource within workspace
			item := m.multiWSSelectedItem()
			if item != nil && m.multiWS.view.NextChange(item.changes) {
				m.multiWS.scroll = 0
				m.multiWS.view.RecomputeCompact(item.output, item.hlOutput, item.changes)
			}
		} else {
			// Normal: next workspace
			if m.multiWS.cursor < len(m.multiWS.items)-1 {
				m.multiWS.cursor++
			} else {
				m.multiWS.cursor = 0
			}
			m.multiWS.scroll = 0
			m.multiWS.view.changeCur = 0
			// Recompute compact diff for new workspace
			if m.multiWS.view.compactDiff {
				if item := m.multiWSSelectedItem(); item != nil {
					m.multiWS.view.RecomputeCompact(item.output, item.hlOutput, item.changes)
				}
			}
		}
		return m, nil

	case "N":
		if m.multiWS.view.focusView {
			// Focus mode: previous resource within workspace
			item := m.multiWSSelectedItem()
			if item != nil && m.multiWS.view.PrevChange(item.changes) {
				m.multiWS.scroll = 0
				m.multiWS.view.RecomputeCompact(item.output, item.hlOutput, item.changes)
			}
		} else {
			// Normal: previous workspace
			if m.multiWS.cursor > 0 {
				m.multiWS.cursor--
			} else {
				m.multiWS.cursor = len(m.multiWS.items) - 1
			}
			m.multiWS.scroll = 0
			m.multiWS.view.changeCur = 0
			// Recompute compact diff for new workspace
			if m.multiWS.view.compactDiff {
				if item := m.multiWSSelectedItem(); item != nil {
					m.multiWS.view.RecomputeCompact(item.output, item.hlOutput, item.changes)
				}
			}
		}
		return m, nil

	case "d", "ctrl+d":
		// Page down in detail output
		maxScroll := m.multiWSMaxScroll()
		m.multiWS.scroll += scrollPageSize
		if m.multiWS.scroll > maxScroll {
			m.multiWS.scroll = maxScroll
		}
		return m, nil

	case "u", "ctrl+u":
		// Page up — disables auto-follow during apply
		m.multiWS.scroll -= scrollPageSize
		if m.multiWS.scroll < 0 {
			m.multiWS.scroll = 0
		}
		m.multiWS.followApply = false
		return m, nil

	case "g":
		m.multiWS.scroll = 0
		m.multiWS.followApply = false
		return m, nil

	case "G":
		m.multiWS.scroll = m.multiWSMaxScroll()
		// Re-enable auto-follow if an apply is in progress
		if item := m.multiWSSelectedItem(); item != nil && item.status == mwsApplying {
			m.multiWS.followApply = true
		}
		return m, nil

	case "y":
		// Apply selected workspace (streaming)
		item := m.multiWSSelectedItem()
		if item != nil && item.status == mwsPlanned {
			idx := m.multiWS.cursor
			m.multiWS.phase = "applying"
			m.prepareItemForApply(idx)
			ws := m.multiWS.items[idx].workspace
			pf := m.multiWS.items[idx].planFile
			return m, m.startMultiWSApplyStream(ws, pf)
		}
		return m, nil

	case "A":
		// Apply all workspaces with changes (sequential)
		hasWork := false
		for i := range m.multiWS.items {
			if m.multiWS.items[i].status == mwsPlanned {
				m.multiWS.items[i].applyQueued = true
				hasWork = true
			}
		}
		if hasWork {
			m.multiWS.phase = "applying"
			cmd := m.startNextMultiWSApply()
			return m, cmd
		}
		return m, nil

	case "?":
		// Could show help but for now just return
		return m, nil
	}

	return m, nil
}

// closeMultiWS exits multi-workspace mode and cleans up plan files.
func (m *Model) closeMultiWS() {
	if m.multiWS.cancel != nil {
		m.multiWS.cancel()
	}
	if m.multiWS.applyCancel != nil {
		m.multiWS.applyCancel()
	}
	for _, item := range m.multiWS.items {
		if item.planFile != "" {
			os.Remove(item.planFile)
		}
	}
	m.multiWS = multiWSState{}
	m.statusMsg = ""
}

// multiWSSelectedItem returns a pointer to the currently selected item.
func (m *Model) multiWSSelectedItem() *multiWSItem {
	if m.multiWS.cursor >= 0 && m.multiWS.cursor < len(m.multiWS.items) {
		return &m.multiWS.items[m.multiWS.cursor]
	}
	return nil
}

// multiWSMaxScroll returns max scroll for the currently selected item.
func (m *Model) multiWSMaxScroll() int {
	item := m.multiWSSelectedItem()
	if item == nil {
		return 0
	}
	return m.multiWS.view.MaxScroll(item.output, item.hlOutput, item.changes, m.multiWSVisibleHeight())
}

// multiWSVisibleHeight returns visible line count in the detail pane.
func (m *Model) multiWSVisibleHeight() int {
	h := m.height - 6 // title bar + detail title + status + help + padding
	if h < 1 {
		return 1
	}
	return h
}

// ─── Rendering ───────────────────────────────────────────

// renderMultiWS renders the full-screen multi-workspace overlay.
func (m Model) renderMultiWS() string {
	// Layout
	leftWidth := m.width * 30 / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	if leftWidth > 50 {
		leftWidth = 50
	}
	rightWidth := m.width - leftWidth - 1 // separator

	contentH := m.height - 3 // title bar + status + help hint

	// ── Title bar ──
	filterInfo := ""
	if m.multiWS.filter != "" {
		filterInfo = " (filter: " + m.multiWS.filter + ")"
	}
	phaseIcon := m.multiWSPhaseIcon()
	titleText := fmt.Sprintf(" ⚡ Multi-Workspace %s%s ", phaseIcon, filterInfo)
	countText := fmt.Sprintf(" %d workspaces ", len(m.multiWS.items))
	titlePad := m.width - lipgloss.Width(titleText) - lipgloss.Width(countText)
	if titlePad < 0 {
		titlePad = 0
	}
	titleBar := ui.PanelTitle.Render(titleText) +
		ui.DimItem.Render(strings.Repeat("─", titlePad)) +
		ui.StatusKey.Render(countText)

	// ── Left column: workspace list ──
	left := m.renderMultiWSList(leftWidth, contentH)

	// ── Separator ──
	sep := lipgloss.NewStyle().
		Foreground(ui.DimGray).
		Render(strings.Repeat("│\n", contentH))

	// ── Right column: selected workspace output ──
	right := m.renderMultiWSDetail(rightWidth, contentH)

	// ── Compose ──
	content := lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right)

	// ── Status bar ──
	statusBar := m.renderMultiWSStatus()

	// ── Help hint ──
	helpHint := m.renderMultiWSHelp()

	return lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		content,
		statusBar,
		helpHint,
	)
}

func (m Model) renderMultiWSList(width, height int) string {
	var lines []string

	for i, item := range m.multiWS.items {
		icon := m.multiWSStatusIcon(item.status)
		label := item.workspace
		badges := changeBadges(item.changes)

		// Truncate label if needed (account for icon + badges)
		badgesWidth := lipgloss.Width(badges)
		maxLabel := width - 6 - badgesWidth // icon + padding + badges
		if maxLabel < 10 {
			maxLabel = 10
		}
		if len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}

		line := fmt.Sprintf(" %s %s", icon, label) + badges

		// Add summary on next line if there's room
		summary := item.summary
		if summary == "" {
			switch item.status {
			case mwsQueued:
				summary = "queued"
			case mwsPlanning:
				summary = "planning..."
			}
		}

		// Pad to width
		lineWidth := lipgloss.Width(line)
		if lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}

		if i == m.multiWS.cursor {
			line = ui.SelectedItem.Render(line)
		}
		lines = append(lines, line)

		// Summary line (indented)
		if summary != "" {
			sumLine := "     " + ui.DimItem.Render(summary)
			sumWidth := lipgloss.Width(sumLine)
			if sumWidth < width {
				sumLine += strings.Repeat(" ", width-sumWidth)
			}
			if i == m.multiWS.cursor {
				sumLine = lipgloss.NewStyle().
					Background(lipgloss.Color("#1A2744")).
					Render(sumLine)
			}
			lines = append(lines, sumLine)
		}
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderMultiWSDetail(width, height int) string {
	item := m.multiWSSelectedItem()
	if item == nil {
		return strings.Repeat(strings.Repeat(" ", width)+"\n", height)
	}

	// Title — show status during apply, resource info in focus mode
	detailTitle := item.workspace
	switch item.status {
	case mwsApplying:
		detailTitle = "⟳ Applying: " + item.workspace
	case mwsApplied:
		detailTitle = "✓ Applied: " + item.workspace
	case mwsApplyFail:
		detailTitle = "✗ Apply Failed: " + item.workspace
	default:
		if c := m.multiWS.view.CurrentChange(item.changes); c != nil && m.multiWS.view.focusView {
			detailTitle = fmt.Sprintf("🔍 [%d/%d] %s%s %s",
				m.multiWS.view.changeCur+1, len(item.changes),
				actionIcon(c.Action), actionLabel(c.Action), c.Address)
		}
	}
	title := ui.PanelTitle.Render(" " + detailTitle + " ")
	titlePad := width - lipgloss.Width(title)
	if titlePad < 0 {
		titlePad = 0
	}
	titleLine := title + ui.DimItem.Render(strings.Repeat("─", titlePad))

	// Content
	visH := height - 1 // title
	if visH < 1 {
		visH = 1
	}

	var sourceLines []string
	var hlLines []string

	if len(item.output) > 0 {
		sourceLines, hlLines = m.multiWS.view.ViewLines(item.output, item.hlOutput, item.changes)
	} else {
		switch item.status {
		case mwsPlanning:
			sourceLines = []string{"", "  ⟳ Planning — waiting for terraform..."}
		default:
			sourceLines = []string{"", "  No output yet"}
		}
	}

	useHL := len(hlLines) == len(sourceLines) && len(hlLines) > 0

	var contentLines []string
	for i := m.multiWS.scroll; i < len(sourceLines) && len(contentLines) < visH; i++ {
		var line string
		if useHL && i < len(hlLines) {
			line = hlLines[i]
		} else {
			line = sourceLines[i]
		}
		wrapped := ui.WrapPlanLines([]string{line}, width)
		for _, wl := range wrapped {
			if len(contentLines) >= visH {
				break
			}
			contentLines = append(contentLines, wl)
		}
	}

	// Pad
	for len(contentLines) < visH {
		contentLines = append(contentLines, "")
	}

	return titleLine + "\n" + strings.Join(contentLines, "\n")
}

func (m Model) renderMultiWSStatus() string {
	// Count statuses
	var planned, failed, noChanges, applied, applyFail, total int
	total = len(m.multiWS.items)
	for _, item := range m.multiWS.items {
		switch item.status {
		case mwsPlanned:
			planned++
		case mwsPlanFail:
			failed++
		case mwsNoChanges:
			noChanges++
		case mwsApplied:
			applied++
		case mwsApplyFail:
			applyFail++
		}
	}

	parts := []string{}
	if planned > 0 {
		parts = append(parts, ui.WarningStyle.Render(fmt.Sprintf("%d with changes", planned)))
	}
	if noChanges > 0 {
		parts = append(parts, ui.SuccessStyle.Render(fmt.Sprintf("%d no changes", noChanges)))
	}
	if applied > 0 {
		parts = append(parts, ui.SuccessStyle.Render(fmt.Sprintf("%d applied", applied)))
	}
	if failed > 0 {
		parts = append(parts, ui.ErrorStyle.Render(fmt.Sprintf("%d failed", failed)))
	}
	if applyFail > 0 {
		parts = append(parts, ui.ErrorStyle.Render(fmt.Sprintf("%d apply failed", applyFail)))
	}
	done := planned + failed + noChanges + applied + applyFail
	progress := fmt.Sprintf("%d/%d", done, total)

	left := ui.StatusKey.Render("progress:") + " " + ui.StatusValue.Render(progress)
	if len(parts) > 0 {
		left += "  " + strings.Join(parts, "  ")
	}

	return ui.StatusBar.Width(m.width).Render(left)
}

func (m Model) renderMultiWSHelp() string {
	var keys []struct{ key, desc string }

	// Dynamic labels based on focus mode
	nLabel := "next/prev workspace"
	focusLabel := "focus"
	if m.multiWS.view.focusView {
		nLabel = "next/prev resource"
		focusLabel = "full plan"
	}
	compactLabel := "compact"
	if m.multiWS.view.compactDiff {
		compactLabel = "full diff"
	}

	switch m.multiWS.phase {
	case "planning":
		keys = []struct{ key, desc string }{
			{"n/N", nLabel},
			{"j/k", "scroll"},
			{"d/u", "page"},
			{"f", focusLabel},
			{"z", compactLabel},
			{"esc", "cancel"},
		}
	case "reviewing":
		keys = []struct{ key, desc string }{
			{"n/N", nLabel},
			{"j/k", "scroll"},
			{"d/u", "page"},
			{"f", focusLabel},
			{"z", compactLabel},
			{"y", "apply selected"},
			{"A", "apply ALL with changes"},
			{"esc", "close"},
		}
	case "applying":
		keys = []struct{ key, desc string }{
			{"n/N", nLabel},
			{"j/k", "scroll"},
			{"d/u", "page"},
			{"f", focusLabel},
			{"z", compactLabel},
			{"esc", "cancel"},
		}
	case "done":
		keys = []struct{ key, desc string }{
			{"n/N", nLabel},
			{"j/k", "scroll"},
			{"d/u", "page"},
			{"f", focusLabel},
			{"z", compactLabel},
			{"esc", "close"},
		}
	}

	var parts []string
	tag := ui.StatusKey.Render("[Multi-WS]")
	parts = append(parts, tag)
	for _, k := range keys {
		parts = append(parts, ui.HelpKey.Render(k.key)+ui.HelpSep.Render(":")+ui.HelpDesc.Render(k.desc))
	}
	return strings.Join(parts, " ")
}

// ─── Helpers ─────────────────────────────────────────────

func (m Model) multiWSPhaseIcon() string {
	switch m.multiWS.phase {
	case "planning":
		return "Planning"
	case "reviewing":
		return "Review"
	case "applying":
		return "Applying"
	case "done":
		return "Done"
	}
	return ""
}

func (m Model) multiWSStatusIcon(status multiWSStatus) string {
	switch status {
	case mwsQueued:
		return ui.DimItem.Render("⏳")
	case mwsPlanning:
		return ui.SpinnerLabel.Render("⟳")
	case mwsPlanned:
		return ui.WarningStyle.Render("⚠")
	case mwsPlanFail:
		return ui.ErrorStyle.Render("✗")
	case mwsNoChanges:
		return ui.SuccessStyle.Render("✓")
	case mwsApplying:
		return ui.SpinnerLabel.Render("⟳")
	case mwsApplied:
		return ui.SuccessStyle.Render("✓")
	case mwsApplyFail:
		return ui.ErrorStyle.Render("✗")
	}
	return "?"
}

// changeBadges builds colored inline badges showing resource change counts.
// Output looks like: [+3] [~1] [-2]  (green, yellow, red)
// Only non-zero action types are shown. Returns "" if no changes.
func changeBadges(changes []planChange) string {
	if len(changes) == 0 {
		return ""
	}

	var creates, updates, destroys, replaces, reads int
	for _, c := range changes {
		switch c.Action {
		case "create":
			creates++
		case "update", "change":
			updates++
		case "destroy", "delete":
			destroys++
		case "replace":
			replaces++
		case "read":
			reads++
		}
	}

	// Badge styles: icon + count, colored per action
	type badge struct {
		icon  string
		count int
		style lipgloss.Style
	}

	badges := []badge{
		{"+", creates, ui.PlanAdd},
		{"~", updates, ui.PlanChange},
		{"±", replaces, ui.PlanChange},
		{"-", destroys, ui.PlanDestroy},
		{"?", reads, ui.PlanInfo},
	}

	var parts []string
	for _, b := range badges {
		if b.count > 0 {
			parts = append(parts, b.style.Render(fmt.Sprintf("%s%d", b.icon, b.count)))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// extractPlanSummary pulls the "Plan: X to add, Y to change, Z to destroy" line
// from terraform plan output.
func extractPlanSummary(lines []string) string {
	for _, line := range lines {
		if strings.Contains(line, "Plan:") && strings.Contains(line, "to add") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// isNoChanges checks if plan output indicates no changes needed.
func isNoChanges(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, "No changes") || strings.Contains(line, "no changes") {
			return true
		}
	}
	return false
}

// collapseRefreshLines replaces consecutive "Refreshing state..." lines with
// a single summary. These lines dominate terraform plan output (one per resource
// in state) but carry no useful information for reviewing plan results.
func collapseRefreshLines(lines []string) []string {
	var result []string
	refreshCount := 0

	flush := func() {
		if refreshCount > 0 {
			summary := fmt.Sprintf("  ··· %d resources refreshed ···", refreshCount)
			result = append(result, ui.DimItem.Render(summary))
			refreshCount = 0
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "Refreshing...") ||
			strings.Contains(trimmed, "Refreshing state...") ||
			strings.Contains(trimmed, "Refresh complete after") ||
			strings.Contains(trimmed, "Reading...") ||
			strings.Contains(trimmed, "Read complete after") {
			refreshCount++
			continue
		}
		flush()
		result = append(result, line)
	}
	flush()
	return result
}



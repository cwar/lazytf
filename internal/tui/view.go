package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cwar/lazytf/internal/ui"
)

// ─── View ────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Multi-workspace overlay (full-screen mode)
	if m.multiWS.active {
		return m.renderMultiWS()
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

	for _, p := range m.panels {
		isActive := p.ID == m.activePanel
		rendered := p.Render(width, isActive, m.focus == FocusLeft)
		parts = append(parts, rendered)
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
	if m.planReview {
		if c := m.planView.CurrentChange(m.planChanges); c != nil {
			modeTag := ""
			if m.planView.focusView {
				modeTag = "🔍 "
			}
			detailTitle = fmt.Sprintf("%s[%d/%d] %s%s %s",
				modeTag, m.planView.changeCur+1, len(m.planChanges),
				actionIcon(c.Action), actionLabel(c.Action), c.Address)
		}
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
		// Plan review: use planView which handles focus + compact
		sourceLines, hlLines = m.planView.ViewLines(m.detailLines, m.highlightedLines, m.planChanges)
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
		if m.planView.focusView {
			focusLabel = "full plan"
		}
		compactLabel := "compact"
		if m.planView.compactDiff {
			compactLabel = "full diff"
		}
		hint := ui.WarningStyle.Render("▶ Review plan") + "  " +
			ui.HelpKey.Render("y") + ui.HelpSep.Render(":") + ui.SuccessStyle.Render(action) + "  " +
			ui.HelpKey.Render("esc") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("cancel") + "  " +
			ui.HelpKey.Render("n/N") + ui.HelpSep.Render(":") + ui.HelpDesc.Render("next/prev") + "  " +
			ui.HelpKey.Render("f") + ui.HelpSep.Render(":") + ui.HelpDesc.Render(focusLabel) + "  " +
			ui.HelpKey.Render("z") + ui.HelpSep.Render(":") + ui.HelpDesc.Render(compactLabel) + "  "
		if m.planView.focusView {
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
	ctxKeys := contextKeysFor(m.activePanel, &m)

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
			{"W", "multi-ws"},
			{"G", "graph"},
			{"f", "fmt"},
			{"l", "log"},
			{"r", "refresh"},
			{"1-6", "panels"},
			{"tab", "next panel"},
			{"?", "help"},
			{"q", "quit"},
		}
	} else {
		globalKeys = []struct{ key, desc string }{
			{"p", "plan"},
			{"a", "apply"},
			{"i", "init"},
			{"D", "destroy"},
			{"W", "multi-ws"},
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
			{"Tab/S-Tab", "Cycle through panels forward/back"},
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
			{"W", "Multi-workspace plan (parallel)"},
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
			{"f", "Toggle focus mode (single resource)"},
			{"z", "Toggle compact diff (collapse unchanged heredocs)"},
			{"c", "Copy current resource diff to clipboard"},
		}},
		{"Multi-Workspace (W)", []struct{ key, desc string }{
			{"n/N", "Next/previous workspace"},
			{"j/k ↑/↓", "Scroll output"},
			{"d/u", "Page down/up"},
			{"g/G", "Top/bottom"},
			{"y", "Apply selected workspace"},
			{"A", "Apply ALL with changes (sequential)"},
			{"esc", "Close / cancel"},
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
			{"/", "Filter workspaces (e.g. dev, prod)"},
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

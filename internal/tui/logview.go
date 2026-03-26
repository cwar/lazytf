package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/ui"
)

// handleLogKey dispatches keys in the command history overlay.
func (m Model) handleLogKey(key string) (tea.Model, tea.Cmd) {
	if m.logView.viewing {
		return m.handleLogDetailKey(key)
	}
	return m.handleLogListKey(key)
}

// handleLogListKey handles keys in the command list view.
func (m Model) handleLogListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc", "l":
		m.showLog = false
		m.logView = logState{}
		m.detailScroll = 0
		return m, nil
	case "j", "down":
		if m.logView.cursor < m.history.len()-1 {
			m.logView.cursor++
		}
		return m, nil
	case "k", "up":
		if m.logView.cursor > 0 {
			m.logView.cursor--
		}
		return m, nil
	case "enter", " ":
		if m.history.len() > 0 {
			m.logView.viewing = true
			m.logView.viewScroll = 0
			m.detailScroll = 0
		}
		return m, nil
	case "g":
		m.logView.cursor = 0
		return m, nil
	case "G":
		if m.history.len() > 0 {
			m.logView.cursor = m.history.len() - 1
		}
		return m, nil
	}
	return m, nil
}

// handleLogDetailKey handles keys when viewing a single command's output.
func (m Model) handleLogDetailKey(key string) (tea.Model, tea.Cmd) {
	rec := m.history.get(m.logView.cursor)
	if rec == nil {
		m.logView.viewing = false
		return m, nil
	}
	max := scrollMax(len(rec.lines), m.detailVisibleHeight())
	switch key {
	case "esc", "q", "l":
		// Return to list
		m.logView.viewing = false
		m.detailScroll = 0
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
	case "g":
		m.scrollToTop()
		return m, nil
	case "G":
		m.scrollToBottom(max)
		return m, nil
	}
	return m, nil
}

// logSourceLines returns the lines (and optional highlighted lines) to display
// in the command history overlay. Handles both the list view and the detail view.
func (m *Model) logSourceLines() ([]string, []string) {
	if m.logView.viewing {
		return m.logDetailLines()
	}
	return m.logListLines(), nil
}

// logListLines renders the command history as a selectable list.
func (m *Model) logListLines() []string {
	if m.history.len() == 0 {
		lines := []string{
			"",
			"  No commands run yet",
			"",
			"  Run a terraform command (p/a/i/v) to see output here",
		}
		if m.isLoading {
			lines = append(lines, "", "  ⟳ "+m.loadingMsg)
		}
		return lines
	}

	var lines []string
	lines = append(lines, "")

	for i := 0; i < m.history.len(); i++ {
		rec := m.history.get(i)
		cursor := "  "
		if i == m.logView.cursor {
			cursor = ui.WarningStyle.Render("▶ ")
		}

		status := ui.SuccessStyle.Render("✓")
		if rec.failed {
			status = ui.ErrorStyle.Render("✗")
		}

		age := formatAge(rec.timestamp)
		ws := rec.workspace
		if ws == "" {
			ws = "default"
		}

		line := fmt.Sprintf("%s%s %s  %s  %s",
			cursor, status, rec.title,
			ui.DimItem.Render(ws),
			ui.DimItem.Render(age))
		lines = append(lines, line)
	}

	if m.isLoading {
		lines = append(lines, "", "  ⟳ "+m.loadingMsg)
	}

	lines = append(lines, "")
	lines = append(lines, "  "+ui.DimItem.Render("enter:view  j/k:navigate  esc:close"))

	return lines
}

// logDetailLines returns the output of the currently selected history entry.
func (m *Model) logDetailLines() ([]string, []string) {
	rec := m.history.get(m.logView.cursor)
	if rec == nil {
		return []string{"", "  No entry selected"}, nil
	}
	return rec.lines, rec.hlLines
}

// formatAge returns a human-readable relative time string.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		return t.Format("Jan 2 15:04")
	}
}

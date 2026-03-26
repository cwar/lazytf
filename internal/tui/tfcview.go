package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/ui"
)

// tfcRunsLoadedMsg is sent when TFC runs have been fetched.
type tfcRunsLoadedMsg struct {
	runs []tfcRunEntry
	err  error
}

// tfcPlanLoadedMsg is sent when a single TFC plan log has been fetched.
type tfcPlanLoadedMsg struct {
	runID  string
	title  string
	output string
	err    error
}

// tfcRunEntry is a history-compatible record from a TFC run.
type tfcRunEntry struct {
	record cmdRecord
	planID string // TFC plan ID for fetching log output
	runID  string // TFC run ID
}

// loadTFCRuns fetches recent runs from Terraform Cloud for the current workspace.
func (m *Model) loadTFCRuns() tea.Cmd {
	if m.tfcClient == nil || !m.tfcClient.HasToken() {
		return nil
	}
	ws := m.workspace
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		runs, err := m.tfcClient.ListRuns(ctx, ws, 20)
		if err != nil {
			return tfcRunsLoadedMsg{err: err}
		}

		var entries []tfcRunEntry
		for _, r := range runs {
			failed := false
			switch r.Status {
			case "errored", "canceled", "force_canceled", "discarded":
				failed = true
			}

			title := "Plan"
			if r.IsDestroy {
				title = "Destroy"
			}
			if strings.Contains(r.Status, "appli") {
				title = "Apply"
			}

			source := r.Source
			switch r.Source {
			case "tfe-ui":
				source = "ui"
			case "tfe-api":
				source = "api"
			case "tfe-cli", "tfe-local":
				source = "cli"
			case "tfe-vcs":
				source = "vcs"
			}

			msg := r.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}

			rec := cmdRecord{
				title:     "☁ " + title,
				workspace: ws,
				timestamp: r.CreatedAt,
				failed:    failed,
				lines:     []string{"", "  ☁ Terraform Cloud Run: " + r.ID, "", "  Status:  " + r.Status, "  Source:  " + source, "  Message: " + msg, "", "  Press enter to fetch full plan output"},
			}

			entries = append(entries, tfcRunEntry{
				record: rec,
				planID: r.PlanID,
				runID:  r.ID,
			})
		}
		return tfcRunsLoadedMsg{runs: entries}
	}
}

// loadTFCPlanLog fetches the plan log for a specific TFC run.
func (m *Model) loadTFCPlanLog(planID, runID, title string) tea.Cmd {
	if m.tfcClient == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, err := m.tfcClient.GetPlanLog(ctx, planID)
		return tfcPlanLoadedMsg{
			runID:  runID,
			title:  title,
			output: output,
			err:    err,
		}
	}
}

// handleTFCRunsLoaded processes fetched TFC runs and adds them to history.
func (m *Model) handleTFCRunsLoaded(msg tfcRunsLoadedMsg) {
	if msg.err != nil {
		m.statusMsg = ui.ErrorStyle.Render("☁ TFC: " + msg.err.Error())
		return
	}

	// Add cloud runs to history (they get the ☁ prefix in title to distinguish)
	for _, entry := range msg.runs {
		m.history.push(entry.record)
	}
	// Store the TFC entries separately so we can look up plan IDs later
	m.tfcRuns = msg.runs

	m.rebuildHistoryPanel()
	m.statusMsg = ui.SuccessStyle.Render(fmt.Sprintf("☁ Loaded %d runs from Terraform Cloud", len(msg.runs)))
}

// handleTFCPlanLoaded processes a fetched TFC plan log.
func (m *Model) handleTFCPlanLoaded(msg tfcPlanLoadedMsg) {
	if msg.err != nil {
		m.statusMsg = ui.ErrorStyle.Render("☁ TFC plan: " + msg.err.Error())
		return
	}

	// Parse and highlight the plan output
	lines := strings.Split(msg.output, "\n")
	hlLines := ui.HighlightPlanOutput(msg.output)
	changes := parsePlanChanges(lines)

	// Update the matching history entry with full output
	for i := 0; i < m.history.len(); i++ {
		rec := m.history.get(i)
		if len(rec.lines) > 1 && strings.Contains(rec.lines[1], msg.runID) {
			rec.lines = lines
			rec.hlLines = hlLines
			rec.changes = changes
			break
		}
	}

	// Refresh the detail pane if viewing history
	if m.activePanel == PanelHistory {
		m.onSelectionChanged()
	}
	m.statusMsg = ui.SuccessStyle.Render("☁ Plan output loaded")
}

// findTFCPlanID looks up the TFC plan ID for a history entry by matching the run ID.
func (m *Model) findTFCPlanID(rec *cmdRecord) (planID, runID string, ok bool) {
	for _, entry := range m.tfcRuns {
		if len(rec.lines) > 1 && strings.Contains(rec.lines[1], entry.runID) {
			return entry.planID, entry.runID, true
		}
	}
	return "", "", false
}

package tui

import "os"

// savePlanState preserves the current plan review state so the user can
// return to it later. Clears the active plan review state but keeps the
// plan file on disk.
func (m *Model) savePlanState() {
	m.lastPlanFile = m.pendingPlanFile
	m.lastPlanIsDestroy = m.planIsDestroy
	m.lastPlanTitle = m.detailTitle

	// Deep-copy slices so later mutations don't corrupt the snapshot
	m.lastPlanLines = make([]string, len(m.detailLines))
	copy(m.lastPlanLines, m.detailLines)

	m.lastPlanHighlighted = make([]string, len(m.highlightedLines))
	copy(m.lastPlanHighlighted, m.highlightedLines)

	m.lastPlanChanges = make([]planChange, len(m.planChanges))
	copy(m.lastPlanChanges, m.planChanges)

	// Clear active plan review state
	m.pendingPlanFile = ""
	m.planReview = false
	m.planIsDestroy = false
	m.planChanges = nil
	m.planView.Reset()
}

// restorePlanState re-enters plan review mode from a previously saved plan.
// Returns true if the plan was successfully restored, false if there is no
// saved plan or the plan file no longer exists on disk.
func (m *Model) restorePlanState() bool {
	if m.lastPlanFile == "" {
		return false
	}

	// Verify the plan file still exists
	if _, err := os.Stat(m.lastPlanFile); err != nil {
		m.clearLastPlan()
		return false
	}

	// Restore into active plan review state
	m.pendingPlanFile = m.lastPlanFile
	m.planReview = true
	m.planIsDestroy = m.lastPlanIsDestroy
	m.planView.Reset()

	m.planChanges = make([]planChange, len(m.lastPlanChanges))
	copy(m.planChanges, m.lastPlanChanges)

	// Restore the detail pane
	m.detailTitle = m.lastPlanTitle
	m.detailLines = make([]string, len(m.lastPlanLines))
	copy(m.detailLines, m.lastPlanLines)
	m.highlightedLines = make([]string, len(m.lastPlanHighlighted))
	copy(m.highlightedLines, m.lastPlanHighlighted)
	m.isHighlighted = true
	m.detailScroll = 0

	// Focus right pane so user can scroll the plan
	m.focus = FocusRight

	// Clear the saved state (now active again)
	m.lastPlanFile = ""
	m.lastPlanIsDestroy = false
	m.lastPlanLines = nil
	m.lastPlanHighlighted = nil
	m.lastPlanChanges = nil
	m.lastPlanTitle = ""

	return true
}

// clearLastPlan removes any saved plan state and deletes the plan file from disk.
func (m *Model) clearLastPlan() {
	if m.lastPlanFile != "" {
		os.Remove(m.lastPlanFile)
	}
	m.lastPlanFile = ""
	m.lastPlanIsDestroy = false
	m.lastPlanLines = nil
	m.lastPlanHighlighted = nil
	m.lastPlanChanges = nil
	m.lastPlanTitle = ""
}

// hasLastPlan returns true if there is a saved plan that can be recalled.
func (m *Model) hasLastPlan() bool {
	return m.lastPlanFile != ""
}

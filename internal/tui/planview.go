package tui

import "github.com/cwar/lazytf/internal/ui"

// ─── Plan Viewer ─────────────────────────────────────────
//
// planViewer manages the display state for reviewing terraform plan output.
// Shared between normal plan review (single workspace) and multi-workspace
// plan review. It handles focus mode, compact diff, and resource navigation.
//
// Scroll offset is managed externally because:
//   - Normal plan review shares m.detailScroll with other pane modes
//   - Multi-ws uses m.multiWS.scroll (independent from the main pane)

// planViewer manages focus, compact diff, and resource navigation for plan output.
type planViewer struct {
	focusView   bool // true = showing one resource at a time
	changeCur   int  // currently selected resource change index
	compactDiff bool // true = collapse unchanged heredoc lines

	// Compact diff cache — invalidated on toggle or navigation
	compactLines []string
	compactHL    []string
}

// Reset clears all view state to defaults.
func (pv *planViewer) Reset() {
	*pv = planViewer{}
}

// FocusBlock extracts the current resource's block from the full output.
// Returns the full output if there are no changes or cursor is out of range.
func (pv *planViewer) FocusBlock(lines, hl []string, changes []planChange) (src []string, hlOut []string) {
	if len(changes) == 0 || pv.changeCur < 0 || pv.changeCur >= len(changes) {
		return lines, hl
	}
	c := changes[pv.changeCur]
	start := c.Line
	end := c.EndLine
	if start > len(lines) {
		start = len(lines)
	}
	if end > len(lines) {
		end = len(lines)
	}
	src = lines[start:end]
	if len(hl) >= end {
		hlOut = hl[start:end]
	}
	return src, hlOut
}

// ViewLines returns the lines to display based on current state
// (focus mode, compact diff). Pass the full plan lines and highlights.
func (pv *planViewer) ViewLines(lines, hl []string, changes []planChange) ([]string, []string) {
	if pv.compactDiff && len(pv.compactLines) > 0 {
		return pv.compactLines, pv.compactHL
	}
	if pv.focusView && len(changes) > 0 {
		return pv.FocusBlock(lines, hl, changes)
	}
	return lines, hl
}

// RecomputeCompact rebuilds the compact diff cache from the current state.
// Must be called whenever compactDiff, focusView, or changeCur changes.
func (pv *planViewer) RecomputeCompact(lines, hl []string, changes []planChange) {
	if !pv.compactDiff {
		pv.compactLines = nil
		pv.compactHL = nil
		return
	}

	source, hlSrc := lines, hl
	if pv.focusView && len(changes) > 0 {
		source, hlSrc = pv.FocusBlock(lines, hl, changes)
	}

	if len(hlSrc) == len(source) {
		pv.compactLines, pv.compactHL = ui.CompactDiffHighlighted(source, hlSrc, 3)
	} else {
		pv.compactLines = ui.CompactDiff(source, 3)
		pv.compactHL = nil
	}
}

// ToggleFocus flips focus mode. Returns true if the toggle happened
// (i.e., there were changes to focus on). Does NOT reset changeCur —
// the caller handles that since normal plan review and multi-ws differ
// in how they reset cursor on focus toggle.
func (pv *planViewer) ToggleFocus(changes []planChange) bool {
	if len(changes) == 0 {
		return false
	}
	pv.focusView = !pv.focusView
	return true
}

// ToggleCompact flips compact diff mode and recomputes the cache.
func (pv *planViewer) ToggleCompact(lines, hl []string, changes []planChange) {
	pv.compactDiff = !pv.compactDiff
	pv.RecomputeCompact(lines, hl, changes)
}

// NextChange advances to the next resource change, wrapping around.
// Returns true if there were changes to navigate.
func (pv *planViewer) NextChange(changes []planChange) bool {
	if len(changes) == 0 {
		return false
	}
	pv.changeCur++
	if pv.changeCur >= len(changes) {
		pv.changeCur = 0
	}
	return true
}

// PrevChange moves to the previous resource change, wrapping around.
// Returns true if there were changes to navigate.
func (pv *planViewer) PrevChange(changes []planChange) bool {
	if len(changes) == 0 {
		return false
	}
	pv.changeCur--
	if pv.changeCur < 0 {
		pv.changeCur = len(changes) - 1
	}
	return true
}

// MaxScroll returns the maximum scroll value given visible height.
func (pv *planViewer) MaxScroll(lines, hl []string, changes []planChange, visibleHeight int) int {
	source, _ := pv.ViewLines(lines, hl, changes)
	max := len(source) - visibleHeight
	if max < 0 {
		return 0
	}
	return max
}

// CurrentChange returns the currently selected change, or nil.
func (pv *planViewer) CurrentChange(changes []planChange) *planChange {
	if pv.changeCur >= 0 && pv.changeCur < len(changes) {
		return &changes[pv.changeCur]
	}
	return nil
}

// CopyBlock returns the raw text of the current resource's block, for clipboard.
func (pv *planViewer) CopyBlock(lines []string, changes []planChange) string {
	src, _ := pv.FocusBlock(lines, nil, changes)
	result := ""
	for i, line := range src {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

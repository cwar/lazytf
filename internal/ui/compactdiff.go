package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FoldMarkerStyle is the style for fold markers showing hidden line counts.
var FoldMarkerStyle = lipgloss.NewStyle().
	Foreground(DimGray).
	Italic(true)

// computeKeepMask classifies each line and returns a boolean mask indicating
// which lines to keep. Lines outside heredocs are always kept. Inside
// heredocs, only diff lines (+/-/~) and `context` surrounding lines are kept,
// plus the first and last line of each heredoc span for readability.
func computeKeepMask(lines []string, context int) []bool {
	h := NewPlanHighlighter()
	kinds := make([]PlanLineKind, len(lines))
	inHeredoc := make([]bool, len(lines))

	for i, line := range lines {
		kinds[i] = h.ClassifyLine(line)
		inHeredoc[i] = h.inHeredoc
	}

	keep := make([]bool, len(lines))
	for i := range lines {
		if !inHeredoc[i] {
			keep[i] = true
			continue
		}
		if kinds[i] == PlanLineAdd || kinds[i] == PlanLineDestroy || kinds[i] == PlanLineChange {
			lo := i - context
			if lo < 0 {
				lo = 0
			}
			hi := i + context
			if hi >= len(lines) {
				hi = len(lines) - 1
			}
			for j := lo; j <= hi; j++ {
				keep[j] = true
			}
		}
	}

	// Keep heredoc boundary lines for readability
	for i := range lines {
		if inHeredoc[i] {
			if i == 0 || !inHeredoc[i-1] {
				keep[i] = true
			}
			if i == len(lines)-1 || !inHeredoc[i+1] {
				keep[i] = true
			}
		}
	}

	return keep
}

// foldMarker builds a styled "··· N lines hidden ···" string aligned with
// the surrounding code indentation.
func foldMarker(lines []string, foldStart, foldEnd, hidden int) string {
	indent := estimateIndent(lines, foldStart, foldEnd)
	return fmt.Sprintf("%s%s", indent,
		FoldMarkerStyle.Render(fmt.Sprintf("··· %d lines hidden ···", hidden)))
}

// CompactDiff collapses unchanged runs inside heredoc blocks, keeping
// `context` lines of context around each actual change (+/-/~).
// Lines outside heredocs are always preserved.
// Returns a new slice of lines suitable for display. The original slice
// is not modified.
func CompactDiff(lines []string, context int) []string {
	if len(lines) == 0 {
		return nil
	}
	if context < 0 {
		context = 0
	}

	keep := computeKeepMask(lines, context)

	var result []string
	i := 0
	for i < len(lines) {
		if keep[i] {
			result = append(result, lines[i])
			i++
			continue
		}
		foldStart := i
		for i < len(lines) && !keep[i] {
			i++
		}
		result = append(result, foldMarker(lines, foldStart, i, i-foldStart))
	}
	return result
}

// CompactDiffHighlighted applies CompactDiff to both the raw lines and their
// corresponding highlighted lines, keeping them in sync. Both slices must
// have the same length.
func CompactDiffHighlighted(lines, highlighted []string, context int) ([]string, []string) {
	if len(lines) == 0 {
		return nil, nil
	}
	if len(highlighted) != len(lines) {
		return CompactDiff(lines, context), nil
	}
	if context < 0 {
		context = 0
	}

	keep := computeKeepMask(lines, context)

	var rawResult, hlResult []string
	i := 0
	for i < len(lines) {
		if keep[i] {
			rawResult = append(rawResult, lines[i])
			hlResult = append(hlResult, highlighted[i])
			i++
			continue
		}
		foldStart := i
		for i < len(lines) && !keep[i] {
			i++
		}
		marker := foldMarker(lines, foldStart, i, i-foldStart)
		rawResult = append(rawResult, marker)
		hlResult = append(hlResult, marker)
	}
	return rawResult, hlResult
}

// estimateIndent guesses the indentation to use for a fold marker by
// looking at the lines immediately before and after the fold.
func estimateIndent(lines []string, foldStart, foldEnd int) string {
	// Look at the line before the fold
	if foldStart > 0 {
		line := lines[foldStart-1]
		trimmed := strings.TrimLeft(line, " \t")
		if len(trimmed) > 0 {
			return line[:len(line)-len(trimmed)]
		}
	}
	// Fallback: look at the first hidden line
	if foldStart < len(lines) {
		line := lines[foldStart]
		trimmed := strings.TrimLeft(line, " \t")
		if len(trimmed) > 0 {
			return line[:len(line)-len(trimmed)]
		}
	}
	return "            " // 12 spaces as sensible default
}

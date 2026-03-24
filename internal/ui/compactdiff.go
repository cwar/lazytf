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

	// Phase 1: classify every line and track heredoc spans
	h := NewPlanHighlighter()
	kinds := make([]PlanLineKind, len(lines))
	inHeredoc := make([]bool, len(lines))

	for i, line := range lines {
		kinds[i] = h.ClassifyLine(line)
		inHeredoc[i] = h.inHeredoc
		// Also mark heredoc start/end lines
		// The line that opens the heredoc (<<-EOT) is NOT in the heredoc per
		// the highlighter (it enters heredoc AFTER classifying), but we still
		// want to keep it. Similarly the EOT line exits heredoc.
	}

	// Phase 2: mark lines as "keep" — everything outside heredocs, plus
	// diff lines and context within heredocs
	keep := make([]bool, len(lines))

	for i := range lines {
		if !inHeredoc[i] {
			// Outside heredoc: always keep (attributes, resource headers, etc.)
			keep[i] = true
			continue
		}
		// Inside heredoc: keep if it's a diff line
		if kinds[i] == PlanLineAdd || kinds[i] == PlanLineDestroy || kinds[i] == PlanLineChange {
			// Mark this line and surrounding context
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

	// Also keep the first and last line of each heredoc (the <<-EOT and EOT lines)
	// These are classified outside the heredoc by the highlighter, so they're
	// already kept. But ensure the first few and last few heredoc content lines
	// are also kept for readability.
	// Find heredoc boundaries and keep 1 line at each end.
	for i := range lines {
		if inHeredoc[i] {
			// First line of a heredoc span
			if i == 0 || !inHeredoc[i-1] {
				keep[i] = true
			}
			// Last line of a heredoc span
			if i == len(lines)-1 || !inHeredoc[i+1] {
				keep[i] = true
			}
		}
	}

	// Phase 3: build output, replacing collapsed runs with fold markers
	var result []string
	i := 0
	for i < len(lines) {
		if keep[i] {
			result = append(result, lines[i])
			i++
			continue
		}

		// Start of a collapsed run — count how many lines to skip
		foldStart := i
		for i < len(lines) && !keep[i] {
			i++
		}
		hidden := i - foldStart

		// Insert a fold marker
		// Use indentation matching the surrounding content for visual alignment
		indent := estimateIndent(lines, foldStart, i)
		marker := fmt.Sprintf("%s%s", indent,
			FoldMarkerStyle.Render(fmt.Sprintf("··· %d lines hidden ···", hidden)))
		result = append(result, marker)
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
		// Fall back to just compacting raw lines if lengths mismatch
		return CompactDiff(lines, context), nil
	}
	if context < 0 {
		context = 0
	}

	// Phase 1: classify every line and track heredoc spans
	h := NewPlanHighlighter()
	kinds := make([]PlanLineKind, len(lines))
	inHeredoc := make([]bool, len(lines))

	for i, line := range lines {
		kinds[i] = h.ClassifyLine(line)
		inHeredoc[i] = h.inHeredoc
	}

	// Phase 2: mark lines as "keep"
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

	// Keep heredoc boundary lines
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

	// Phase 3: build output for both raw and highlighted
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
		hidden := i - foldStart

		indent := estimateIndent(lines, foldStart, i)
		marker := fmt.Sprintf("%s%s", indent,
			FoldMarkerStyle.Render(fmt.Sprintf("··· %d lines hidden ···", hidden)))
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

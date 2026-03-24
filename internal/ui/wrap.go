package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// WrapPlanLines soft-wraps each line in the slice to fit within `width`
// visible columns. Uses ANSI-aware wrapping that preserves escape codes
// and re-emits active styles on continuation lines so highlighting is
// not lost at wrap boundaries.
// Returns a new slice where each element is one visual line.
func WrapPlanLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if width < 10 {
		width = 10 // minimum sane width
	}

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		visWidth := ansi.StringWidth(line)
		if visWidth <= width {
			result = append(result, line)
			continue
		}

		// Wrap using ANSI-aware hard wrap. preserveSpace=true keeps
		// indentation on continuation lines.
		wrapped := ansi.Hardwrap(line, width, true)
		parts := strings.Split(wrapped, "\n")

		// Hardwrap doesn't re-emit ANSI codes on continuation lines.
		// Track active style cumulatively and prepend to each continuation.
		activeStyle := ""
		for i, part := range parts {
			if i > 0 && activeStyle != "" {
				part = activeStyle + part
			}
			result = append(result, part)
			// Update active style from the original (unstyled) part.
			// Only change activeStyle if this part actually contains SGR codes;
			// otherwise the previous style carries forward.
			if newStyle, found := extractActiveANSICarry(parts[i]); found {
				activeStyle = newStyle
			}
		}
	}
	return result
}

// extractActiveANSI scans a string and returns the ANSI SGR state that
// is active at the end of the string. If a reset (\x1b[0m) is the last
// SGR sequence, returns "". This is a convenience wrapper for tests.
func extractActiveANSI(s string) string {
	style, _ := extractActiveANSICarry(s)
	return style
}

// extractActiveANSICarry scans a string for ANSI SGR sequences and returns
// the state active at the end. The bool indicates whether any SGR codes were
// found at all — if false, the caller should carry forward the previous style.
func extractActiveANSICarry(s string) (string, bool) {
	var lastSGR string
	found := false
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			start := i
			i += 2
			for i < len(s) && !isCSITerminator(s[i]) {
				i++
			}
			if i < len(s) {
				i++ // include terminator
				seq := s[start:i]
				// Only track SGR sequences (terminated by 'm')
				if seq[len(seq)-1] == 'm' {
					found = true
					if seq == "\x1b[0m" || seq == "\x1b[m" {
						lastSGR = "" // reset clears active style
					} else {
						lastSGR = seq
					}
				}
			}
		} else {
			i++
		}
	}
	return lastSGR, found
}

// isCSITerminator returns true if the byte terminates a CSI sequence.
func isCSITerminator(b byte) bool {
	return b >= 0x40 && b <= 0x7E // '@' through '~'
}

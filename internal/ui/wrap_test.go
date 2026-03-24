package ui

import (
	"strings"
	"testing"
)

func TestWrapPlanLines_ShortLinesUnchanged(t *testing.T) {
	lines := []string{
		"  # resource will be updated",
		"  ~ resource \"test\" {",
		"      ~ attr = \"short\"",
		"    }",
	}
	result := WrapPlanLines(lines, 80)
	if len(result) != len(lines) {
		t.Errorf("Short lines should not be wrapped: got %d, want %d", len(result), len(lines))
	}
	for i, l := range result {
		if l != lines[i] {
			t.Errorf("Line %d changed: %q -> %q", i, lines[i], l)
		}
	}
}

func TestWrapPlanLines_LongLineWraps(t *testing.T) {
	long := "        druid.segmentCache.locations=[{\"path\":\"/druid/data/segments\",\"maxSize\":\"10035Gi\"}]"
	lines := []string{long}
	result := WrapPlanLines(lines, 40)

	if len(result) <= 1 {
		t.Error("Long line should have been wrapped into multiple lines")
	}

	// Rejoining should give back the original content (minus any added newlines)
	rejoined := strings.Join(result, "")
	// Remove any leading spaces from continuation lines
	if !strings.Contains(rejoined, "druid.segmentCache.locations") {
		t.Error("Wrapped output should contain the original content")
	}

	// No visual line should exceed the width
	for i, l := range result {
		// Use rune count as approximation (no ANSI in this test)
		if len(l) > 60 { // some margin for wrap continuation indent
			t.Errorf("Wrapped line %d too long (%d chars): %q", i, len(l), l)
		}
	}
}

func TestWrapPlanLines_PreservesANSI(t *testing.T) {
	// Simulate a green highlighted line (PlanAdd style) that's too long
	line := "\x1b[32m          +       podDisruptionBudgetSpec: some really long value that extends beyond the pane width limit here\x1b[0m"
	result := WrapPlanLines([]string{line}, 50)

	if len(result) <= 1 {
		t.Fatal("Long ANSI line should have been wrapped")
	}

	// Every continuation line should have the green ANSI code re-emitted
	for i, part := range result {
		if !strings.Contains(part, "\x1b[32m") {
			t.Errorf("Line %d missing green ANSI code — highlighting lost on wrap:\n  %q", i, part)
		}
	}

	// Content should be preserved
	full := strings.Join(result, "\n")
	if !strings.Contains(full, "podDisruptionBudgetSpec") {
		t.Error("Content should be preserved after wrapping")
	}
}

func TestWrapPlanLines_ResetDoesNotLeak(t *testing.T) {
	// A line that ends with reset should NOT apply style to next logical line
	line1 := "\x1b[31m- removed: this is a long line that needs wrapping to fit in the pane\x1b[0m"
	line2 := "    unchanged content here"
	result := WrapPlanLines([]string{line1, line2}, 40)

	// The last entry should be the unchanged line with no ANSI
	last := result[len(result)-1]
	if strings.Contains(last, "\x1b[") {
		t.Errorf("Unchanged line after reset should have no ANSI codes: %q", last)
	}
}

func TestExtractActiveANSI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"\x1b[32mhello", "\x1b[32m"},                   // green, no reset
		{"\x1b[32mhello\x1b[0m", ""},                     // green then reset
		{"\x1b[1m\x1b[32mhello", "\x1b[32m"},             // bold then green — last wins
		{"no ansi here", ""},                              // plain text
		{"\x1b[38;5;196mred text", "\x1b[38;5;196m"},     // 256-color
		{"\x1b[32mstart\x1b[0m\x1b[33mcontinue", "\x1b[33m"}, // reset then yellow
	}

	for _, tt := range tests {
		got := extractActiveANSI(tt.input)
		if got != tt.want {
			t.Errorf("extractActiveANSI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWrapPlanLines_EmptyInput(t *testing.T) {
	result := WrapPlanLines(nil, 80)
	if len(result) != 0 {
		t.Errorf("nil input should return nil, got %d lines", len(result))
	}

	result = WrapPlanLines([]string{}, 80)
	if len(result) != 0 {
		t.Errorf("empty input should return empty, got %d lines", len(result))
	}
}

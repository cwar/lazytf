package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestStreamBuffer_LinesAccumulateDuringStream verifies that streaming lines
// are buffered even when the user isn't viewing the stream.
func TestStreamBuffer_LinesAccumulateDuringStream(t *testing.T) {
	m := testModel()

	// Start a streaming command
	result, _ := m.Update(cmdStartMsg{title: "Plan"})
	m = result.(Model)

	// Receive some streaming lines
	ch := make(chan string, 10)
	var cmdErr error
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "line1", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "line2", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)

	// Buffer should have the lines
	if len(m.streamLines) != 2 {
		t.Fatalf("streamLines should have 2 lines, got %d", len(m.streamLines))
	}
	if m.streamLines[0] != "line1" || m.streamLines[1] != "line2" {
		t.Errorf("streamLines = %v", m.streamLines)
	}
}

// TestStreamBuffer_NavigateAwayPreservesBuffer verifies that navigating to a
// file doesn't destroy the command output buffer.
func TestStreamBuffer_NavigateAwayPreservesBuffer(t *testing.T) {
	m := testModel()

	// Start streaming
	result, _ := m.Update(cmdStartMsg{title: "Plan"})
	m = result.(Model)

	ch := make(chan string, 10)
	var cmdErr error
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "plan line 1", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "plan line 2", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)

	if !m.viewingStream {
		t.Fatal("should be viewing stream initially")
	}

	// Navigate away — simulate onSelectionChanged replacing detailLines
	m.onSelectionChanged()

	if m.viewingStream {
		t.Fatal("should NOT be viewing stream after navigation")
	}

	// Buffer must still have the plan output
	if len(m.streamLines) != 2 {
		t.Fatalf("streamLines should still have 2 lines after navigation, got %d", len(m.streamLines))
	}
}

// TestStreamBuffer_NavigateAwayStopsAppendingToDetail verifies that streaming
// lines don't corrupt the file content after navigation.
func TestStreamBuffer_NavigateAwayStopsAppendingToDetail(t *testing.T) {
	m := testModel()

	// Start streaming
	result, _ := m.Update(cmdStartMsg{title: "Plan"})
	m = result.(Model)

	ch := make(chan string, 10)
	var cmdErr error
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "plan line 1", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)

	// Navigate away — file content replaces detail
	m.detailLines = []string{"resource aws_instance {", "  ami = var.ami", "}"}
	m.viewingStream = false

	// More streaming lines arrive
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "plan line 2", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)

	// detailLines should NOT have the streaming line appended
	if len(m.detailLines) != 3 {
		t.Fatalf("detailLines should still have 3 file lines, got %d: %v", len(m.detailLines), m.detailLines)
	}
	// But buffer should have both plan lines
	if len(m.streamLines) != 2 {
		t.Fatalf("streamLines should have 2 lines, got %d", len(m.streamLines))
	}
}

// TestStreamBuffer_BKeyRestoresOutput verifies pressing 'b' while a command
// is running restores the streaming output to the detail pane.
func TestStreamBuffer_BKeyRestoresOutput(t *testing.T) {
	m := testModel()

	// Start streaming
	result, _ := m.Update(cmdStartMsg{title: "Plan"})
	m = result.(Model)

	ch := make(chan string, 10)
	var cmdErr error
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "plan output 1", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "plan output 2", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)

	// Navigate away
	m.detailLines = []string{"some file content"}
	m.viewingStream = false

	// More streaming output arrives in buffer
	result, _ = m.Update(cmdStreamLineMsg{title: "Plan", line: "plan output 3", ch: ch, cmdErr: &cmdErr})
	m = result.(Model)

	// Press 'b' to go back to stream
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = result.(Model)

	if !m.viewingStream {
		t.Fatal("should be viewing stream after pressing b")
	}
	if len(m.detailLines) != 3 {
		t.Fatalf("detailLines should have 3 stream lines, got %d: %v", len(m.detailLines), m.detailLines)
	}
	if m.detailLines[0] != "plan output 1" {
		t.Errorf("expected plan output, got %q", m.detailLines[0])
	}
}

// TestStreamBuffer_BKeyOnlyWorksWhenLoading verifies 'b' is a no-op when
// no command is running (falls through to normal handling).
func TestStreamBuffer_BKeyOnlyWorksWhenLoading(t *testing.T) {
	m := testModel()
	m.isLoading = false
	m.viewingStream = false
	m.focus = FocusLeft

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = result.(Model)

	// Should not change viewingStream — 'b' is not a command when idle
	if m.viewingStream {
		t.Fatal("b should not activate stream view when not loading")
	}
}

// TestStreamBuffer_CmdDoneRestoresBuffer verifies that when a command
// finishes, the buffer is copied to detailLines even if user navigated away.
func TestStreamBuffer_CmdDoneRestoresBuffer(t *testing.T) {
	m := testModel()
	m.isLoading = true

	// Simulate buffer with streamed output
	m.streamLines = []string{"plan line 1", "plan line 2", "plan line 3"}
	m.streamHLLines = []string{"hl1", "hl2", "hl3"}
	m.viewingStream = false
	// detailLines is showing some file
	m.detailLines = []string{"file content here"}

	doneMsg := cmdDoneMsg{title: "Plan", err: nil, streamed: true}
	result, _ := m.Update(doneMsg)
	updated := result.(Model)

	// detailLines should now show the plan output, not the file
	if len(updated.detailLines) != 3 {
		t.Fatalf("detailLines should have 3 plan lines, got %d: %v", len(updated.detailLines), updated.detailLines)
	}
	if updated.detailLines[0] != "plan line 1" {
		t.Errorf("expected plan line 1, got %q", updated.detailLines[0])
	}
}

// TestStreamBuffer_FollowOutputReEnabledOnB verifies pressing 'b' re-enables
// auto-scroll so the user sees the latest output.
func TestStreamBuffer_FollowOutputReEnabledOnB(t *testing.T) {
	m := testModel()
	m.isLoading = true
	m.streamLines = make([]string, 50)
	m.streamHLLines = make([]string, 50)
	for i := range m.streamLines {
		m.streamLines[i] = "line"
		m.streamHLLines[i] = "line"
	}
	m.viewingStream = false
	m.followOutput = false
	m.focus = FocusLeft

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = result.(Model)

	if !m.followOutput {
		t.Fatal("followOutput should be re-enabled when pressing b")
	}
	if m.focus != FocusRight {
		t.Fatal("focus should move to right pane when pressing b")
	}
}

// TestStreamBuffer_HelpHintShowsBKey verifies the help hint shows 'b:output'
// when a command is running and user is browsing away.
func TestStreamBuffer_HelpHintShowsBKey(t *testing.T) {
	m := testModel()
	m.width = 120
	m.height = 40
	m.isLoading = true
	m.viewingStream = false
	m.streamLines = []string{"some output"}
	m.loadingMsg = "Plan..."

	hint := m.renderHelpHint()
	if hint == "" {
		t.Fatal("help hint should not be empty")
	}
	// The hint should show 'b:output' for switching back to stream
	if !containsText(hint, "output") {
		t.Errorf("help hint should mention 'output', got: %q", hint)
	}
}

// TestStreamBuffer_HelpHintHiddenWhenViewing verifies the 'b:output' hint
// does NOT appear when already viewing the stream.
func TestStreamBuffer_HelpHintHiddenWhenViewing(t *testing.T) {
	m := testModel()
	m.width = 120
	m.height = 40
	m.isLoading = true
	m.viewingStream = true
	m.streamLines = []string{"some output"}
	m.loadingMsg = "Plan..."

	hint := m.renderHelpHint()
	// Should NOT show 'b:output' — already viewing it
	if containsText(hint, "b:output") {
		t.Errorf("help hint should NOT show 'b:output' when viewing stream, got: %q", hint)
	}
}

// containsText checks if text appears in a string (ignoring ANSI codes).
func containsText(s, substr string) bool {
	// Simple: just check the raw string — ANSI codes don't interfere with
	// single-char lookups.
	return len(s) > 0 && len(substr) > 0 && contains(s, substr)
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

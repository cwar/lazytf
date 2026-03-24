package tui

import (
	"os/exec"
	"testing"
)

func TestClipboardCmd_FindsSomething(t *testing.T) {
	// On any reasonable dev machine, at least one clipboard tool should exist.
	// This test documents the detection order rather than being a strict requirement.
	cmd := clipboardCmd()
	if cmd == nil {
		t.Skip("no clipboard command available on this system")
	}
	// Verify it's one of our expected tools
	base := cmd.Path
	found := false
	for _, name := range []string{"wl-copy", "xclip", "xsel", "pbcopy"} {
		if path, err := exec.LookPath(name); err == nil && base == path {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("clipboardCmd returned unexpected command: %s", base)
	}
}

func TestCopyToClipboard_ReturnsMsg(t *testing.T) {
	cmd := clipboardCmd()
	if cmd == nil {
		t.Skip("no clipboard command available")
	}

	fn := copyToClipboard("test content")
	msg := fn()

	cbMsg, ok := msg.(clipboardMsg)
	if !ok {
		t.Fatalf("expected clipboardMsg, got %T", msg)
	}
	if cbMsg.err != nil {
		t.Errorf("clipboard copy failed: %v", cbMsg.err)
	}
}

func TestErrNoClipboard_Message(t *testing.T) {
	err := errNoClipboard
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestPlanReview_CKeyCopiesResource(t *testing.T) {
	m := basePlanReviewModel()
	m.planFocusView = true
	m.planChangeCur = 0

	// The 'c' key should return a non-nil command (the clipboard cmd)
	updated := sendKey(m, "c")

	// Status should show the "copying" message
	if updated.statusMsg == "" {
		t.Error("statusMsg should be set after pressing 'c'")
	}
}

func TestPlanReview_CKeyNoChanges(t *testing.T) {
	m := basePlanReviewModel()
	m.planChanges = nil // no resources

	updated := sendKey(m, "c")

	// Should be a no-op, no status change
	if updated.statusMsg != "" {
		t.Errorf("statusMsg should be empty when no resources, got %q", updated.statusMsg)
	}
}

func TestClipboardMsg_Success(t *testing.T) {
	m := basePlanReviewModel()
	m.planChangeCur = 0

	updated, _ := m.Update(clipboardMsg{err: nil})
	model := updated.(Model)

	if model.statusMsg == "" {
		t.Error("statusMsg should show success after clipboard copy")
	}
}

func TestClipboardMsg_Error(t *testing.T) {
	m := basePlanReviewModel()

	updated, _ := m.Update(clipboardMsg{err: &clipboardError{"test error"}})
	model := updated.(Model)

	if model.statusMsg == "" {
		t.Error("statusMsg should show error")
	}
}

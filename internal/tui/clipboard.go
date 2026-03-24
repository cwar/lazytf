package tui

import (
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// clipboardMsg is sent after a clipboard copy attempt completes.
type clipboardMsg struct {
	err error
}

// copyToClipboard returns a tea.Cmd that writes text to the system clipboard.
// It tries platform-specific clipboard commands in order of preference.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		cmd := clipboardCmd()
		if cmd == nil {
			return clipboardMsg{err: errNoClipboard}
		}
		cmd.Stdin = strings.NewReader(text)
		return clipboardMsg{err: cmd.Run()}
	}
}

// errNoClipboard is returned when no clipboard command is found.
var errNoClipboard = &clipboardError{"no clipboard command found (install wl-copy, xclip, or pbcopy)"}

type clipboardError struct{ msg string }

func (e *clipboardError) Error() string { return e.msg }

// clipboardCmd returns an *exec.Cmd for the first available clipboard tool,
// or nil if none are found.
func clipboardCmd() *exec.Cmd {
	// Wayland
	if path, err := exec.LookPath("wl-copy"); err == nil {
		return exec.Command(path)
	}
	// X11
	if path, err := exec.LookPath("xclip"); err == nil {
		return exec.Command(path, "-selection", "clipboard")
	}
	if path, err := exec.LookPath("xsel"); err == nil {
		return exec.Command(path, "--clipboard", "--input")
	}
	// macOS
	if path, err := exec.LookPath("pbcopy"); err == nil {
		return exec.Command(path)
	}
	return nil
}

package tui

import "github.com/cwar/lazytf/internal/ui"

// isBusy returns true when a terraform command is currently running.
// Used to prevent accidental duplicate/concurrent operations.
func (m *Model) isBusy() bool {
	return m.isLoading
}

// busyMsg returns the standard status bar message shown when a key is blocked.
func busyMsg() string {
	return ui.WarningStyle.Render("⏳ Command in progress — wait for it to finish")
}

// isOperationKey returns true if the key triggers a terraform operation that
// must not run concurrently. These keys are blocked when isBusy() is true.
func isOperationKey(key string) bool {
	switch key {
	case "a", "p", "i", "v", "f", "F", "D", "P", "r", "R":
		return true
	}
	return false
}

// isContextOperationKey returns true for context-specific keys (on the
// Resources panel) that trigger terraform state-mutating operations.
func isContextOperationKey(panel PanelID, key string) bool {
	switch panel {
	case PanelResources:
		switch key {
		case "t", "u", "x", "T":
			return true
		}
	}
	return false
}

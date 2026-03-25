package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/terraform"
)

// ─── Picker Activation ───────────────────────────────────

func TestWSPicker_ShowsOnFilterSubmit(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)

	m.startMultiWS("dev")

	if !m.multiWS.active {
		t.Fatal("expected multi-ws to be active")
	}
	if m.multiWS.phase != "selecting" {
		t.Errorf("expected phase 'selecting', got %q", m.multiWS.phase)
	}
}

func TestWSPicker_ContainsFilteredWorkspaces(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4", "podcast-dev"},
		[]string{"default"},
		nil,
	)

	m.startMultiWS("dev")

	picker := m.multiWS.picker
	// "dev" matches: dev-gew4, dev-gae2, podcast-dev (3 workspaces)
	if len(picker.workspaces) != 3 {
		t.Fatalf("expected 3 workspaces, got %d: %v", len(picker.workspaces), picker.workspaces)
	}
}

func TestWSPicker_AllSelectedByDefault(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)

	m.startMultiWS("")

	picker := m.multiWS.picker
	for i, ws := range picker.workspaces {
		if !picker.checked[i] {
			t.Errorf("workspace %q should be selected by default", ws)
		}
	}
}

func TestWSPicker_EmptyFilterShowsAll(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)

	m.startMultiWS("")

	// Should have all non-ignored workspaces
	picker := m.multiWS.picker
	if len(picker.workspaces) != 3 { // minus "default" which is ignored
		t.Fatalf("expected 3 workspaces, got %d: %v", len(picker.workspaces), picker.workspaces)
	}
}

func TestWSPicker_RespectsIgnoreList(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "prod-gew4"},
		[]string{"default", "prod-gew4"},
		nil,
	)

	m.startMultiWS("")

	picker := m.multiWS.picker
	for _, ws := range picker.workspaces {
		if ws == "default" || ws == "prod-gew4" {
			t.Errorf("ignored workspace %q should not appear in picker", ws)
		}
	}
}

func TestWSPicker_ResolvesGroupFilter(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "podcast-dev"},
		[]string{"default"},
		map[string]string{"dev": "dev-"}, // "dev" → "dev-"
	)

	m.startMultiWS("dev")

	// "dev" resolves to "dev-", so only dev-gew4 and dev-gae2 match
	picker := m.multiWS.picker
	if len(picker.workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d: %v", len(picker.workspaces), picker.workspaces)
	}
}

func TestWSPicker_NoMatchShowsError(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)

	m.startMultiWS("staging")

	if m.multiWS.active {
		t.Fatal("should not activate picker with no matching workspaces")
	}
	if !strings.Contains(m.statusMsg, "No workspaces match") {
		t.Errorf("expected no-match message, got %q", m.statusMsg)
	}
}

func TestWSPicker_MatchesVarFiles(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.files = []terraform.TfFile{
		{Name: "dev-gew4.tfvars", Path: "/tmp/dev-gew4.tfvars", IsVars: true},
	}

	m.startMultiWS("")

	picker := m.multiWS.picker
	for i, ws := range picker.workspaces {
		if ws == "dev-gew4" && picker.varFiles[i] != "/tmp/dev-gew4.tfvars" {
			t.Errorf("expected var file for dev-gew4, got %q", picker.varFiles[i])
		}
	}
}

// ─── Picker Key Handling ─────────────────────────────────

func TestWSPicker_SpaceTogglesSelection(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// First item should be checked
	if !m.multiWS.picker.checked[0] {
		t.Fatal("expected first item checked")
	}

	// Press space to uncheck
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = result.(Model)

	if m.multiWS.picker.checked[0] {
		t.Error("expected first item unchecked after space")
	}

	// Press space again to re-check
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = result.(Model)

	if !m.multiWS.picker.checked[0] {
		t.Error("expected first item re-checked after second space")
	}
}

func TestWSPicker_TabTogglesSelection(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Tab should also toggle (fzf convention)
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)

	if m.multiWS.picker.checked[0] {
		t.Error("expected first item unchecked after tab")
	}
}

func TestWSPicker_ATogglesAll(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// All should start checked
	for i := range m.multiWS.picker.checked {
		if !m.multiWS.picker.checked[i] {
			t.Fatalf("expected all checked initially")
		}
	}

	// 'a' toggles all off (since all are currently on)
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = result.(Model)

	for i, ws := range m.multiWS.picker.workspaces {
		if m.multiWS.picker.checked[i] {
			t.Errorf("expected %q unchecked after toggle-all", ws)
		}
	}

	// 'a' again toggles all back on
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = result.(Model)

	for i, ws := range m.multiWS.picker.workspaces {
		if !m.multiWS.picker.checked[i] {
			t.Errorf("expected %q checked after second toggle-all", ws)
		}
	}
}

func TestWSPicker_ATogglesAll_PartialSelection(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Uncheck first item
	m.multiWS.picker.checked[0] = false

	// 'a' should check all (since not all are checked)
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = result.(Model)

	for i, ws := range m.multiWS.picker.workspaces {
		if !m.multiWS.picker.checked[i] {
			t.Errorf("expected %q checked after toggle-all from partial", ws)
		}
	}
}

func TestWSPicker_JKNavigates(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	if m.multiWS.picker.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.multiWS.picker.cursor)
	}

	// j moves down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = result.(Model)
	if m.multiWS.picker.cursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", m.multiWS.picker.cursor)
	}

	// k moves up
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = result.(Model)
	if m.multiWS.picker.cursor != 0 {
		t.Errorf("expected cursor 0 after k, got %d", m.multiWS.picker.cursor)
	}

	// k at top wraps to bottom (fzf-style)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = result.(Model)
	expected := len(m.multiWS.picker.workspaces) - 1
	if m.multiWS.picker.cursor != expected {
		t.Errorf("expected cursor to wrap to %d, got %d", expected, m.multiWS.picker.cursor)
	}
}

func TestWSPicker_JWrapsAround(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Move to last item
	m.multiWS.picker.cursor = len(m.multiWS.picker.workspaces) - 1

	// j at bottom wraps to top
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = result.(Model)
	if m.multiWS.picker.cursor != 0 {
		t.Errorf("expected cursor to wrap to 0, got %d", m.multiWS.picker.cursor)
	}
}

func TestWSPicker_KWrapsAround(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Cursor at top
	if m.multiWS.picker.cursor != 0 {
		t.Fatalf("expected cursor at 0")
	}

	// k at top wraps to bottom
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = result.(Model)

	expected := len(m.multiWS.picker.workspaces) - 1
	if m.multiWS.picker.cursor != expected {
		t.Errorf("expected cursor to wrap to %d, got %d", expected, m.multiWS.picker.cursor)
	}
}

// ─── Picker Confirmation ────────────────────────────────

func TestWSPicker_EnterStartsPlanning(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	if m.multiWS.phase != "selecting" {
		t.Fatalf("expected selecting phase, got %q", m.multiWS.phase)
	}

	// Press enter to confirm
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	if m.multiWS.phase != "planning" {
		t.Errorf("expected phase 'planning', got %q", m.multiWS.phase)
	}
	if len(m.multiWS.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(m.multiWS.items))
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (batch plans)")
	}
}

func TestWSPicker_EnterWithNoneSelectedShowsError(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Uncheck all
	for i := range m.multiWS.picker.checked {
		m.multiWS.picker.checked[i] = false
	}

	// Press enter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	// Should stay in selecting phase with an error
	if m.multiWS.phase != "selecting" {
		t.Errorf("expected still in 'selecting', got %q", m.multiWS.phase)
	}
	if !strings.Contains(m.statusMsg, "No workspaces selected") {
		t.Errorf("expected error message, got %q", m.statusMsg)
	}
}

func TestWSPicker_EnterOnlyPlansSelected(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	// Uncheck the second workspace
	m.multiWS.picker.checked[1] = false

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	if len(m.multiWS.items) != 2 {
		t.Fatalf("expected 2 items (unchecked 1 of 3), got %d", len(m.multiWS.items))
	}

	// Verify the unchecked workspace is not in items
	uncheckedWS := m.multiWS.picker.workspaces[1]
	for _, item := range m.multiWS.items {
		if item.workspace == uncheckedWS {
			t.Errorf("unchecked workspace %q should not be in items", uncheckedWS)
		}
	}
}

func TestWSPicker_EscCancels(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	if !m.multiWS.active {
		t.Fatal("expected active")
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = result.(Model)

	if m.multiWS.active {
		t.Error("expected picker closed after esc")
	}
}

func TestWSPicker_QCancels(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = result.(Model)

	if m.multiWS.active {
		t.Error("expected picker closed after q")
	}
}

// ─── Picker Rendering ───────────────────────────────────

func TestWSPicker_RenderShowsWorkspaces(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("dev")

	output := m.renderMultiWS()

	if !strings.Contains(output, "dev-gew4") {
		t.Error("expected render to contain 'dev-gew4'")
	}
	if !strings.Contains(output, "dev-gae2") {
		t.Error("expected render to contain 'dev-gae2'")
	}
	// prod-gew4 doesn't match "dev" filter
	if strings.Contains(output, "prod-gew4") {
		t.Error("did not expect 'prod-gew4' in filtered picker")
	}
}

func TestWSPicker_RenderShowsSelectionCount(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2", "prod-gew4"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	output := m.renderMultiWS()
	// Should show something like "3/3 selected" or "3 selected"
	if !strings.Contains(output, "3") {
		t.Error("expected render to contain selection count")
	}
}

func TestWSPicker_RenderShowsCheckboxes(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	output := m.renderMultiWS()

	// Should have checkbox-like indicators
	if !strings.Contains(output, "✓") && !strings.Contains(output, "[x]") && !strings.Contains(output, "●") {
		t.Error("expected render to contain checked indicator")
	}
}

func TestWSPicker_RenderShowsVarFiles(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4"},
		[]string{"default"},
		nil,
	)
	m.files = []terraform.TfFile{
		{Name: "dev-gew4.tfvars", Path: "/tmp/dev-gew4.tfvars", IsVars: true},
	}
	m.startMultiWS("")

	output := m.renderMultiWS()

	if !strings.Contains(output, "dev-gew4.tfvars") {
		t.Error("expected render to show matched var file")
	}
}

func TestWSPicker_RenderShowsFilter(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("dev")

	output := m.renderMultiWS()

	if !strings.Contains(output, "dev") {
		t.Error("expected render to show filter")
	}
}

func TestWSPicker_HelpHintShowsPickerKeys(t *testing.T) {
	m := multiWSModel(
		[]string{"default", "dev-gew4", "dev-gae2"},
		[]string{"default"},
		nil,
	)
	m.startMultiWS("")

	output := m.renderMultiWS()

	if !strings.Contains(output, "toggle") {
		t.Error("expected help to mention 'toggle'")
	}
	if !strings.Contains(output, "enter") || !strings.Contains(output, "confirm") {
		t.Error("expected help to mention enter/confirm")
	}
}

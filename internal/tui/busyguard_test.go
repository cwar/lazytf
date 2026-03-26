package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cwar/lazytf/internal/terraform"
)

func dummyResource(addr string) terraform.Resource {
	return terraform.Resource{
		Type:    "aws_instance",
		Name:    "test",
		Address: addr,
	}
}

// baseBusyModel returns a model with isLoading=true to simulate a running command.
func baseBusyModel() Model {
	m := Model{
		width:     120,
		height:    30,
		isLoading: true,
		panels:    makePanels(),
	}
	return m
}

// --- Blocked keys: terraform commands should NOT fire while busy ---

func TestBusy_AKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message warning the user")
	}
}

func TestBusy_PKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message warning the user")
	}
}

func TestBusy_DKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message warning the user")
	}
}

func TestBusy_InitKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message warning the user")
	}
}

func TestBusy_ValidateKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message warning the user")
	}
}

func TestBusy_FmtKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_FmtFixKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_ProvidersKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_RefreshKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_RecallKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	m.lastPlanFile = "/tmp/plan.tfplan"
	m.lastPlanLines = []string{"line"}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command to be dispatched while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

// --- Allowed keys: navigation and overlays should still work while busy ---

func TestBusy_HelpOverlayAllowed(t *testing.T) {
	m := baseBusyModel()
	got := sendKey(m, "?")
	if !got.showHelp {
		t.Fatal("expected help overlay to toggle while busy")
	}
}

func TestBusy_HistoryPanelAllowed(t *testing.T) {
	m := baseBusyModel()
	got := sendKey(m, "l")
	if got.activePanel != PanelHistory {
		t.Fatal("expected l to jump to history panel while busy")
	}
}

func TestBusy_QuitAllowed(t *testing.T) {
	m := baseBusyModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected ctrl+c to return a quit command even while busy")
	}
}

func TestBusy_TabAllowed(t *testing.T) {
	m := baseBusyModel()
	m.activePanel = PanelFiles
	m.runner = terraform.NewRunner("/tmp")
	got := sendSpecialKey(m, tea.KeyTab)
	if got.activePanel != PanelResources {
		t.Fatal("expected tab to cycle panels while busy")
	}
}

func TestBusy_PanelSwitchAllowed(t *testing.T) {
	m := baseBusyModel()
	m.activePanel = PanelStatus
	got := sendKey(m, "3")
	if got.activePanel != PanelResources {
		t.Fatal("expected panel switch to work while busy")
	}
}

func TestBusy_ScrollingAllowed(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusRight
	m.detailLines = make([]string, 100)
	m.detailScroll = 0
	got := sendKey(m, "j")
	if got.detailScroll != 1 {
		t.Fatal("expected scrolling to work while busy")
	}
}

func TestBusy_LeftPaneNavigationAllowed(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusLeft
	m.activePanel = PanelResources
	// Add items so navigation has somewhere to go
	m.panels[PanelResources].Items = []PanelItem{
		{Label: "res1"},
		{Label: "res2"},
	}
	got := sendKey(m, "j")
	if got.panels[PanelResources].Cursor != 1 {
		t.Fatal("expected left pane navigation to work while busy")
	}
}

// --- Context keys that trigger operations should be blocked ---

func TestBusy_TaintKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusLeft
	m.activePanel = PanelResources
	m.panels[PanelResources].Items = []PanelItem{
		{Label: "aws_instance.test", Data: dummyResource("aws_instance.test")},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected taint to be blocked while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_UntaintKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusLeft
	m.activePanel = PanelResources
	m.panels[PanelResources].Items = []PanelItem{
		{Label: "aws_instance.test", Data: dummyResource("aws_instance.test")},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected untaint to be blocked while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_TargetedPlanKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusLeft
	m.activePanel = PanelResources
	m.panels[PanelResources].Items = []PanelItem{
		{Label: "aws_instance.test", Data: dummyResource("aws_instance.test")},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected targeted plan to be blocked while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_StateRmKeyBlocked(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusLeft
	m.activePanel = PanelResources
	m.panels[PanelResources].Items = []PanelItem{
		{Label: "aws_instance.test", Data: dummyResource("aws_instance.test")},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected state rm to be blocked while busy")
	}
	// Should not even show the confirm dialog
	if got.showConfirm {
		t.Fatal("should not show confirm dialog while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_WorkspaceSelectBlocked(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusLeft
	m.activePanel = PanelWorkspaces
	m.workspace = "default"
	m.panels[PanelWorkspaces].Items = []PanelItem{
		{Label: "staging", Data: "staging"},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected workspace select to be blocked while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

// --- Right pane command keys blocked ---

func TestBusy_RightPaneApplyBlocked(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusRight
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected right pane apply to be blocked while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

func TestBusy_RightPanePlanBlocked(t *testing.T) {
	m := baseBusyModel()
	m.focus = FocusRight
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected right pane plan to be blocked while busy")
	}
	if got.statusMsg == "" {
		t.Fatal("expected a status message")
	}
}

// --- Not busy: operations should work normally ---

func TestNotBusy_OperationsAllowed(t *testing.T) {
	m := baseBusyModel()
	m.isLoading = false // not busy
	m.focus = FocusLeft
	// These should not set a "busy" status message
	got := sendKey(m, "r")
	if got.statusMsg != "" && got.statusMsg == busyMsg() {
		t.Fatal("refresh should be allowed when not busy")
	}
}

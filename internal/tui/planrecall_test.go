package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavePlanState(t *testing.T) {
	m := Model{
		pendingPlanFile: "/tmp/test-plan.tfplan",
		planReview:      true,
		planIsDestroy:   false,
		planChanges: []planChange{
			{Address: "aws_instance.web", Action: "create", Line: 5, EndLine: 20},
			{Address: "aws_s3_bucket.logs", Action: "destroy", Line: 22, EndLine: 35},
		},
		planChangeCur: 1,
		planFocusView: true,
		detailLines:   []string{"line1", "line2", "line3"},
		highlightedLines: []string{"hl1", "hl2", "hl3"},
		detailTitle:      "Plan → Apply",
	}

	m.savePlanState()

	// Plan review state should be cleared
	if m.planReview {
		t.Error("planReview should be false after save")
	}
	if m.pendingPlanFile != "" {
		t.Error("pendingPlanFile should be empty after save")
	}
	if m.planChanges != nil {
		t.Error("planChanges should be nil after save")
	}
	if m.planFocusView {
		t.Error("planFocusView should be false after save")
	}

	// Last plan state should be preserved
	if m.lastPlanFile != "/tmp/test-plan.tfplan" {
		t.Errorf("lastPlanFile = %q, want %q", m.lastPlanFile, "/tmp/test-plan.tfplan")
	}
	if m.lastPlanIsDestroy {
		t.Error("lastPlanIsDestroy should be false")
	}
	if len(m.lastPlanLines) != 3 {
		t.Errorf("lastPlanLines length = %d, want 3", len(m.lastPlanLines))
	}
	if len(m.lastPlanHighlighted) != 3 {
		t.Errorf("lastPlanHighlighted length = %d, want 3", len(m.lastPlanHighlighted))
	}
	if len(m.lastPlanChanges) != 2 {
		t.Errorf("lastPlanChanges length = %d, want 2", len(m.lastPlanChanges))
	}
	if m.lastPlanTitle != "Plan → Apply" {
		t.Errorf("lastPlanTitle = %q, want %q", m.lastPlanTitle, "Plan → Apply")
	}
}

func TestSavePlanState_Destroy(t *testing.T) {
	m := Model{
		pendingPlanFile: "/tmp/test-destroy.tfplan",
		planReview:      true,
		planIsDestroy:   true,
		planChanges:     []planChange{{Address: "aws_instance.web", Action: "destroy", Line: 5, EndLine: 20}},
		detailLines:     []string{"destroying..."},
		highlightedLines: []string{"hl-destroying..."},
		detailTitle:      "Plan → Destroy",
	}

	m.savePlanState()

	if !m.lastPlanIsDestroy {
		t.Error("lastPlanIsDestroy should be true for destroy plans")
	}
	if m.lastPlanFile != "/tmp/test-destroy.tfplan" {
		t.Errorf("lastPlanFile = %q, want %q", m.lastPlanFile, "/tmp/test-destroy.tfplan")
	}
}

func TestRestorePlanState(t *testing.T) {
	// Create a temp file to represent the plan file
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "test.tfplan")
	os.WriteFile(planFile, []byte("fake plan"), 0644)

	m := Model{
		lastPlanFile:        planFile,
		lastPlanIsDestroy:   false,
		lastPlanLines:       []string{"line1", "line2", "line3"},
		lastPlanHighlighted: []string{"hl1", "hl2", "hl3"},
		lastPlanChanges: []planChange{
			{Address: "aws_instance.web", Action: "create", Line: 5, EndLine: 20},
		},
		lastPlanTitle: "Plan → Apply",
		// Some other state that should change
		detailTitle:      "something else",
		detailLines:      []string{"other content"},
		highlightedLines: []string{"other hl"},
		focus:            FocusLeft,
	}

	ok := m.restorePlanState()

	if !ok {
		t.Fatal("restorePlanState should return true when plan file exists")
	}
	if !m.planReview {
		t.Error("planReview should be true after restore")
	}
	if m.pendingPlanFile != planFile {
		t.Errorf("pendingPlanFile = %q, want %q", m.pendingPlanFile, planFile)
	}
	if m.planIsDestroy {
		t.Error("planIsDestroy should be false")
	}
	if len(m.planChanges) != 1 {
		t.Errorf("planChanges length = %d, want 1", len(m.planChanges))
	}
	if m.planChangeCur != 0 {
		t.Error("planChangeCur should be reset to 0")
	}
	if m.planFocusView {
		t.Error("planFocusView should be false after restore")
	}
	if m.focus != FocusRight {
		t.Error("focus should be FocusRight after restore")
	}
	// Detail pane should be restored
	if m.detailTitle != "Plan → Apply" {
		t.Errorf("detailTitle = %q, want %q", m.detailTitle, "Plan → Apply")
	}
	if len(m.detailLines) != 3 {
		t.Errorf("detailLines length = %d, want 3", len(m.detailLines))
	}
	if len(m.highlightedLines) != 3 {
		t.Errorf("highlightedLines length = %d, want 3", len(m.highlightedLines))
	}
	if m.isHighlighted != true {
		t.Error("isHighlighted should be true after restore")
	}

	// Last plan state should be cleared
	if m.lastPlanFile != "" {
		t.Error("lastPlanFile should be empty after restore")
	}
	if m.lastPlanLines != nil {
		t.Error("lastPlanLines should be nil after restore")
	}
}

func TestRestorePlanState_MissingFile(t *testing.T) {
	m := Model{
		lastPlanFile:  "/tmp/nonexistent-plan.tfplan",
		lastPlanLines: []string{"line1"},
		lastPlanTitle: "Plan → Apply",
	}

	ok := m.restorePlanState()

	if ok {
		t.Error("restorePlanState should return false when plan file is missing")
	}
	// Last plan state should be cleared since the file is gone
	if m.lastPlanFile != "" {
		t.Error("lastPlanFile should be cleared when file is missing")
	}
}

func TestRestorePlanState_NoSavedPlan(t *testing.T) {
	m := Model{}

	ok := m.restorePlanState()

	if ok {
		t.Error("restorePlanState should return false when no plan saved")
	}
}

func TestClearLastPlan(t *testing.T) {
	// Create a temp file to represent the plan file
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "test.tfplan")
	os.WriteFile(planFile, []byte("fake plan"), 0644)

	m := Model{
		lastPlanFile:        planFile,
		lastPlanIsDestroy:   true,
		lastPlanLines:       []string{"line1"},
		lastPlanHighlighted: []string{"hl1"},
		lastPlanChanges:     []planChange{{Address: "a.b", Action: "create"}},
		lastPlanTitle:       "Plan",
	}

	m.clearLastPlan()

	if m.lastPlanFile != "" {
		t.Error("lastPlanFile should be empty")
	}
	if m.lastPlanIsDestroy {
		t.Error("lastPlanIsDestroy should be false")
	}
	if m.lastPlanLines != nil {
		t.Error("lastPlanLines should be nil")
	}
	if m.lastPlanHighlighted != nil {
		t.Error("lastPlanHighlighted should be nil")
	}
	if m.lastPlanChanges != nil {
		t.Error("lastPlanChanges should be nil")
	}
	if m.lastPlanTitle != "" {
		t.Error("lastPlanTitle should be empty")
	}

	// File should be deleted
	if _, err := os.Stat(planFile); !os.IsNotExist(err) {
		t.Error("plan file should be deleted")
	}
}

func TestHasLastPlan(t *testing.T) {
	m := Model{}
	if m.hasLastPlan() {
		t.Error("hasLastPlan should be false when no plan saved")
	}

	m.lastPlanFile = "/tmp/some-plan.tfplan"
	if !m.hasLastPlan() {
		t.Error("hasLastPlan should be true when plan file is set")
	}
}

func TestSaveThenRestore_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "roundtrip.tfplan")
	os.WriteFile(planFile, []byte("fake plan"), 0644)

	m := Model{
		pendingPlanFile: planFile,
		planReview:      true,
		planIsDestroy:   true,
		planChanges: []planChange{
			{Address: "aws_instance.web", Action: "destroy", Line: 5, EndLine: 20},
			{Address: "aws_s3_bucket.logs", Action: "destroy", Line: 22, EndLine: 35},
		},
		planChangeCur:    1,
		planFocusView:    true,
		detailLines:      []string{"plan output line 1", "plan output line 2"},
		highlightedLines: []string{"hl line 1", "hl line 2"},
		detailTitle:      "Plan → Destroy",
	}

	// Save
	m.savePlanState()

	// Simulate user navigating away
	m.detailLines = []string{"some other file content"}
	m.highlightedLines = []string{"other hl"}
	m.detailTitle = "main.tf"
	m.focus = FocusLeft

	// Restore
	ok := m.restorePlanState()
	if !ok {
		t.Fatal("restore should succeed")
	}

	// Verify full round trip
	if !m.planReview {
		t.Error("planReview should be true")
	}
	if !m.planIsDestroy {
		t.Error("planIsDestroy should be true")
	}
	if m.pendingPlanFile != planFile {
		t.Errorf("pendingPlanFile = %q, want %q", m.pendingPlanFile, planFile)
	}
	if len(m.planChanges) != 2 {
		t.Errorf("planChanges length = %d, want 2", len(m.planChanges))
	}
	if m.detailTitle != "Plan → Destroy" {
		t.Errorf("detailTitle = %q, want %q", m.detailTitle, "Plan → Destroy")
	}
	if len(m.detailLines) != 2 || m.detailLines[0] != "plan output line 1" {
		t.Error("detailLines not properly restored")
	}
	if m.focus != FocusRight {
		t.Error("focus should be FocusRight")
	}
}

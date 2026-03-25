package tui

import (
	"strings"
	"testing"
)

func sampleChanges() []planChange {
	return []planChange{
		{Address: "aws_instance.web", Action: "create", Line: 2, EndLine: 6},
		{Address: "aws_instance.api", Action: "update", Line: 8, EndLine: 12},
		{Address: "aws_instance.old", Action: "destroy", Line: 14, EndLine: 18},
	}
}

func sampleLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "  line " + string(rune('A'+i%26))
	}
	return lines
}

// ─── FocusBlock ──────────────────────────────────────────

func TestPlanViewer_FocusBlock_ExtractsRange(t *testing.T) {
	pv := &planViewer{changeCur: 0}
	lines := sampleLines(20)
	hl := sampleLines(20)
	changes := sampleChanges()

	src, hlOut := pv.FocusBlock(lines, hl, changes)
	if len(src) != 4 { // lines 2..6
		t.Errorf("expected 4 lines, got %d", len(src))
	}
	if len(hlOut) != 4 {
		t.Errorf("expected 4 hl lines, got %d", len(hlOut))
	}
}

func TestPlanViewer_FocusBlock_NoChanges(t *testing.T) {
	pv := &planViewer{changeCur: 0}
	lines := sampleLines(10)
	hl := sampleLines(10)

	src, hlOut := pv.FocusBlock(lines, hl, nil)
	if len(src) != 10 {
		t.Errorf("expected all lines returned when no changes, got %d", len(src))
	}
	if len(hlOut) != 10 {
		t.Errorf("expected all hl returned, got %d", len(hlOut))
	}
}

func TestPlanViewer_FocusBlock_CursorOutOfRange(t *testing.T) {
	pv := &planViewer{changeCur: 5} // out of range
	lines := sampleLines(20)
	changes := sampleChanges()

	src, _ := pv.FocusBlock(lines, nil, changes)
	if len(src) != 20 {
		t.Errorf("expected all lines when cursor OOB, got %d", len(src))
	}
}

// ─── ToggleFocus ─────────────────────────────────────────

func TestPlanViewer_ToggleFocus_On(t *testing.T) {
	pv := &planViewer{changeCur: 2}
	ok := pv.ToggleFocus(sampleChanges())
	if !ok {
		t.Fatal("expected toggle to succeed")
	}
	if !pv.focusView {
		t.Error("expected focusView true")
	}
	// changeCur is NOT reset by ToggleFocus — caller handles that
	if pv.changeCur != 2 {
		t.Errorf("expected changeCur unchanged at 2, got %d", pv.changeCur)
	}
}

func TestPlanViewer_ToggleFocus_Off(t *testing.T) {
	pv := &planViewer{focusView: true, changeCur: 1}
	ok := pv.ToggleFocus(sampleChanges())
	if !ok {
		t.Fatal("expected toggle to succeed")
	}
	if pv.focusView {
		t.Error("expected focusView false")
	}
}

func TestPlanViewer_ToggleFocus_NoChanges(t *testing.T) {
	pv := &planViewer{}
	ok := pv.ToggleFocus(nil)
	if ok {
		t.Error("expected toggle to fail with no changes")
	}
	if pv.focusView {
		t.Error("should not enable focus with no changes")
	}
}

// ─── NextChange / PrevChange ─────────────────────────────

func TestPlanViewer_NextChange_Advances(t *testing.T) {
	pv := &planViewer{changeCur: 0}
	ok := pv.NextChange(sampleChanges())
	if !ok {
		t.Fatal("expected success")
	}
	if pv.changeCur != 1 {
		t.Errorf("expected 1, got %d", pv.changeCur)
	}
}

func TestPlanViewer_NextChange_Wraps(t *testing.T) {
	pv := &planViewer{changeCur: 2}
	pv.NextChange(sampleChanges())
	if pv.changeCur != 0 {
		t.Errorf("expected wrap to 0, got %d", pv.changeCur)
	}
}

func TestPlanViewer_PrevChange_Wraps(t *testing.T) {
	pv := &planViewer{changeCur: 0}
	pv.PrevChange(sampleChanges())
	if pv.changeCur != 2 {
		t.Errorf("expected wrap to 2, got %d", pv.changeCur)
	}
}

func TestPlanViewer_NextChange_Empty(t *testing.T) {
	pv := &planViewer{changeCur: 0}
	ok := pv.NextChange(nil)
	if ok {
		t.Error("expected false for empty changes")
	}
	if pv.changeCur != 0 {
		t.Errorf("expected no change, got %d", pv.changeCur)
	}
}

func TestPlanViewer_PrevChange_Empty(t *testing.T) {
	pv := &planViewer{changeCur: 0}
	ok := pv.PrevChange(nil)
	if ok {
		t.Error("expected false for empty changes")
	}
}

// ─── ToggleCompact ───────────────────────────────────────

func TestPlanViewer_ToggleCompact_On(t *testing.T) {
	pv := &planViewer{}
	lines := []string{
		"  # aws_instance.web will be created",
		`  + resource "aws_instance" "web" {`,
		"      + ami           = <<-EOT",
		"          line 1",
		"          line 2",
		"          line 3",
		"          line 4",
		"          line 5",
		"          line 6",
		"          line 7",
		"          line 8",
		"          line 9",
		"          line 10",
		"        EOT",
		"    }",
	}
	hl := make([]string, len(lines))
	copy(hl, lines)

	pv.ToggleCompact(lines, hl, nil)
	if !pv.compactDiff {
		t.Fatal("expected compactDiff true")
	}
	if pv.compactLines == nil {
		t.Fatal("expected compactLines to be computed")
	}
}

func TestPlanViewer_ToggleCompact_Off(t *testing.T) {
	pv := &planViewer{compactDiff: true, compactLines: []string{"cached"}}
	pv.ToggleCompact(nil, nil, nil)
	if pv.compactDiff {
		t.Fatal("expected compactDiff false")
	}
	if pv.compactLines != nil {
		t.Error("expected compactLines cleared")
	}
}

// ─── ViewLines ───────────────────────────────────────────

func TestPlanViewer_ViewLines_Normal(t *testing.T) {
	pv := &planViewer{}
	lines := sampleLines(10)
	hl := sampleLines(10)

	src, hlOut := pv.ViewLines(lines, hl, nil)
	if len(src) != 10 {
		t.Errorf("expected all lines, got %d", len(src))
	}
	if len(hlOut) != 10 {
		t.Errorf("expected all hl, got %d", len(hlOut))
	}
}

func TestPlanViewer_ViewLines_Focus(t *testing.T) {
	pv := &planViewer{focusView: true, changeCur: 1}
	lines := sampleLines(20)
	hl := sampleLines(20)
	changes := sampleChanges()

	src, _ := pv.ViewLines(lines, hl, changes)
	// Change 1: lines 8..12 = 4 lines
	if len(src) != 4 {
		t.Errorf("expected 4 focused lines, got %d", len(src))
	}
}

func TestPlanViewer_ViewLines_Compact(t *testing.T) {
	pv := &planViewer{
		compactDiff:  true,
		compactLines: []string{"compacted"},
		compactHL:    []string{"compacted-hl"},
	}
	lines := sampleLines(20)
	hl := sampleLines(20)

	src, hlOut := pv.ViewLines(lines, hl, nil)
	if len(src) != 1 || src[0] != "compacted" {
		t.Errorf("expected compact lines, got %v", src)
	}
	if len(hlOut) != 1 || hlOut[0] != "compacted-hl" {
		t.Errorf("expected compact hl, got %v", hlOut)
	}
}

// ─── MaxScroll ───────────────────────────────────────────

func TestPlanViewer_MaxScroll(t *testing.T) {
	pv := &planViewer{}
	lines := sampleLines(50)
	max := pv.MaxScroll(lines, nil, nil, 20)
	if max != 30 {
		t.Errorf("expected max scroll 30, got %d", max)
	}
}

func TestPlanViewer_MaxScroll_Short(t *testing.T) {
	pv := &planViewer{}
	lines := sampleLines(5)
	max := pv.MaxScroll(lines, nil, nil, 20)
	if max != 0 {
		t.Errorf("expected max scroll 0 for short content, got %d", max)
	}
}

// ─── CurrentChange ───────────────────────────────────────

func TestPlanViewer_CurrentChange(t *testing.T) {
	pv := &planViewer{changeCur: 1}
	changes := sampleChanges()
	c := pv.CurrentChange(changes)
	if c == nil {
		t.Fatal("expected non-nil change")
	}
	if c.Address != "aws_instance.api" {
		t.Errorf("expected api, got %s", c.Address)
	}
}

func TestPlanViewer_CurrentChange_OutOfRange(t *testing.T) {
	pv := &planViewer{changeCur: 10}
	c := pv.CurrentChange(sampleChanges())
	if c != nil {
		t.Error("expected nil for out of range")
	}
}

// ─── CopyBlock ───────────────────────────────────────────

func TestPlanViewer_CopyBlock(t *testing.T) {
	pv := &planViewer{changeCur: 0}
	lines := sampleLines(20)
	changes := sampleChanges()

	text := pv.CopyBlock(lines, changes)
	parts := strings.Split(text, "\n")
	if len(parts) != 4 { // lines 2..6
		t.Errorf("expected 4 lines in copy, got %d", len(parts))
	}
}

// ─── Reset ───────────────────────────────────────────────

func TestPlanViewer_Reset(t *testing.T) {
	pv := &planViewer{
		focusView:    true,
		changeCur:    5,
		compactDiff:  true,
		compactLines: []string{"x"},
		compactHL:    []string{"y"},
	}
	pv.Reset()

	if pv.focusView || pv.changeCur != 0 || pv.compactDiff {
		t.Error("expected all fields reset")
	}
	if pv.compactLines != nil || pv.compactHL != nil {
		t.Error("expected caches cleared")
	}
}

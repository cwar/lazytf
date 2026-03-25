package tui

import "testing"

func TestScrollMax(t *testing.T) {
	tests := []struct {
		total, vis, want int
	}{
		{100, 20, 80},
		{10, 20, 0},  // fewer lines than visible → 0
		{20, 20, 0},  // exactly equal → 0
		{0, 20, 0},
		{21, 20, 1},
	}
	for _, tc := range tests {
		got := scrollMax(tc.total, tc.vis)
		if got != tc.want {
			t.Errorf("scrollMax(%d, %d) = %d, want %d", tc.total, tc.vis, got, tc.want)
		}
	}
}

func scrollModel(lines int, scroll int) Model {
	m := Model{
		height:       24, // detailVisibleHeight = 24 - 4 = 20
		detailScroll: scroll,
		detailLines:  make([]string, lines),
	}
	return m
}

func TestScrollDown_Increments(t *testing.T) {
	m := scrollModel(100, 0)
	m.scrollDown(80)
	if m.detailScroll != 1 {
		t.Errorf("got %d, want 1", m.detailScroll)
	}
}

func TestScrollDown_ClampsToMax(t *testing.T) {
	m := scrollModel(100, 80)
	m.scrollDown(80)
	if m.detailScroll != 80 {
		t.Errorf("got %d, want 80 (clamped)", m.detailScroll)
	}
}

func TestScrollUp_Decrements(t *testing.T) {
	m := scrollModel(100, 5)
	m.scrollUp()
	if m.detailScroll != 4 {
		t.Errorf("got %d, want 4", m.detailScroll)
	}
}

func TestScrollUp_ClampsToZero(t *testing.T) {
	m := scrollModel(100, 0)
	m.scrollUp()
	if m.detailScroll != 0 {
		t.Errorf("got %d, want 0", m.detailScroll)
	}
}

func TestScrollPageDown_Increments(t *testing.T) {
	m := scrollModel(100, 0)
	m.scrollPageDown(80)
	if m.detailScroll != scrollPageSize {
		t.Errorf("got %d, want %d", m.detailScroll, scrollPageSize)
	}
}

func TestScrollPageDown_ClampsToMax(t *testing.T) {
	m := scrollModel(100, 70)
	m.scrollPageDown(80)
	if m.detailScroll != 80 {
		t.Errorf("got %d, want 80 (clamped)", m.detailScroll)
	}
}

func TestScrollPageUp_Decrements(t *testing.T) {
	m := scrollModel(100, 20)
	m.scrollPageUp()
	if m.detailScroll != 5 {
		t.Errorf("got %d, want 5", m.detailScroll)
	}
}

func TestScrollPageUp_ClampsToZero(t *testing.T) {
	m := scrollModel(100, 3)
	m.scrollPageUp()
	if m.detailScroll != 0 {
		t.Errorf("got %d, want 0 (clamped)", m.detailScroll)
	}
}

func TestScrollToTop(t *testing.T) {
	m := scrollModel(100, 50)
	m.scrollToTop()
	if m.detailScroll != 0 {
		t.Errorf("got %d, want 0", m.detailScroll)
	}
}

func TestScrollToBottom(t *testing.T) {
	m := scrollModel(100, 0)
	m.scrollToBottom(80)
	if m.detailScroll != 80 {
		t.Errorf("got %d, want 80", m.detailScroll)
	}
}

func TestDetailMaxScroll(t *testing.T) {
	m := scrollModel(100, 0)
	got := m.detailMaxScroll()
	if got != 80 {
		t.Errorf("got %d, want 80", got)
	}
}

func TestDetailMaxScroll_ShortContent(t *testing.T) {
	m := scrollModel(5, 0)
	got := m.detailMaxScroll()
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

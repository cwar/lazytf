package tui

// ─── Scroll Helpers ──────────────────────────────────────
//
// Shared scroll primitives used by overlay, apply-result, plan-review,
// and right-pane key handlers. Each function mutates m.detailScroll
// and clamps to valid bounds.

const scrollPageSize = 15

// scrollMax returns the maximum scroll offset for the given total line
// count and visible height. Never negative.
func scrollMax(totalLines, visibleHeight int) int {
	max := totalLines - visibleHeight
	if max < 0 {
		return 0
	}
	return max
}

// scrollDown increments the scroll offset by 1, clamping to max.
func (m *Model) scrollDown(max int) {
	if m.detailScroll < max {
		m.detailScroll++
	}
}

// scrollUp decrements the scroll offset by 1, clamping to 0.
func (m *Model) scrollUp() {
	if m.detailScroll > 0 {
		m.detailScroll--
	}
}

// scrollPageDown increments the scroll offset by a page, clamping to max.
func (m *Model) scrollPageDown(max int) {
	m.detailScroll += scrollPageSize
	if m.detailScroll > max {
		m.detailScroll = max
	}
}

// scrollPageUp decrements the scroll offset by a page, clamping to 0.
func (m *Model) scrollPageUp() {
	m.detailScroll -= scrollPageSize
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}
}

// scrollToTop sets the scroll offset to 0.
func (m *Model) scrollToTop() {
	m.detailScroll = 0
}

// scrollToBottom sets the scroll offset to max.
func (m *Model) scrollToBottom(max int) {
	m.detailScroll = max
}

// detailMaxScroll returns the max scroll value for the current detail pane
// content (using m.detailLines).
func (m *Model) detailMaxScroll() int {
	return scrollMax(len(m.detailLines), m.detailVisibleHeight())
}

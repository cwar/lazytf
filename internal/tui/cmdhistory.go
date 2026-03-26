package tui

import (
	"time"
)

// maxHistoryEntries is the number of command records to keep in the ring buffer.
const maxHistoryEntries = 50

// cmdRecord stores the output and metadata of a completed terraform command.
type cmdRecord struct {
	title     string       // e.g. "Plan", "Apply", "Plan → Apply"
	workspace string       // workspace the command ran in
	timestamp time.Time    // when the command completed
	failed    bool         // true if the command returned an error
	lines     []string     // raw output lines
	hlLines   []string     // highlighted output lines (ANSI)
	changes   []planChange // parsed resource changes (plan output only)
}

// cmdHistory is a ring buffer of recent command records, newest first.
type cmdHistory struct {
	entries []cmdRecord
}

// push adds a record to the front of the history, capping at maxHistoryEntries.
func (h *cmdHistory) push(r cmdRecord) {
	h.entries = append([]cmdRecord{r}, h.entries...)
	if len(h.entries) > maxHistoryEntries {
		h.entries = h.entries[:maxHistoryEntries]
	}
}

// len returns the number of records in the history.
func (h *cmdHistory) len() int {
	return len(h.entries)
}

// get returns the record at the given index (0 = most recent).
func (h *cmdHistory) get(i int) *cmdRecord {
	if i < 0 || i >= len(h.entries) {
		return nil
	}
	return &h.entries[i]
}



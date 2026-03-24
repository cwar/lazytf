package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// detectGitBranch returns the current git branch for the given directory.
// It first tries reading .git/HEAD directly (fast, no subprocess), then
// falls back to `git rev-parse` for worktrees or unusual layouts.
// Returns "" if not in a git repository.
func detectGitBranch(dir string) string {
	// Fast path: read .git/HEAD directly
	if branch := readGitHead(dir); branch != "" {
		return branch
	}

	// Slow path: shell out to git (handles worktrees, submodules, etc.)
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// readGitHead reads .git/HEAD to extract the branch name.
// Returns "" if the file doesn't exist or HEAD is detached.
func readGitHead(dir string) string {
	head := filepath.Join(dir, ".git", "HEAD")
	data, err := os.ReadFile(head)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	// Normal branch: "ref: refs/heads/main"
	const prefix = "ref: refs/heads/"
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	// Detached HEAD — return short SHA
	if len(s) >= 8 {
		return s[:8]
	}
	return ""
}

package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGitHead_Branch(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.Mkdir(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/cool-thing\n"), 0644)

	got := readGitHead(dir)
	if got != "feature/cool-thing" {
		t.Errorf("readGitHead() = %q, want %q", got, "feature/cool-thing")
	}
}

func TestReadGitHead_Detached(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.Mkdir(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("abc1234567890def\n"), 0644)

	got := readGitHead(dir)
	if got != "abc12345" {
		t.Errorf("readGitHead() = %q, want %q", got, "abc12345")
	}
}

func TestReadGitHead_NoGit(t *testing.T) {
	dir := t.TempDir()
	got := readGitHead(dir)
	if got != "" {
		t.Errorf("readGitHead() = %q, want empty", got)
	}
}

func TestDetectGitBranch_RealRepo(t *testing.T) {
	// This test runs from within the lazytf repo itself
	branch := detectGitBranch("../..")
	if branch == "" {
		t.Skip("not running inside a git repo")
	}
	// Just verify it returned something non-empty
	t.Logf("detected branch: %s", branch)
}

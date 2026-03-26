package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cwar/lazytf/internal/tui"
)

const version = "0.1.0"

func main() {
	// Parse args
	args := os.Args[1:]

	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		case "--version", "-v":
			fmt.Printf("lazytf v%s\n", version)
			os.Exit(0)
		}
	}

	// Determine working directory
	workDir := "."
	if len(args) > 0 {
		workDir = args[0]
	}

	absDir, err := filepath.Abs(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check if directory exists
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a valid directory\n", absDir)
		os.Exit(1)
	}

	// Check for .tf files
	hasTf := false
	entries, _ := os.ReadDir(absDir)
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".tf" {
			hasTf = true
			break
		}
	}
	if !hasTf {
		fmt.Fprintf(os.Stderr, "Warning: No .tf files found in %s\n", absDir)
		fmt.Fprintf(os.Stderr, "Continuing anyway...\n\n")
	}

	// Clean up stale plan files from previous crashed sessions
	cleanStalePlanFiles()

	// Create and run TUI
	model := tui.NewModel(absDir)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`⚡ lazytf — A lazygit-style TUI for Terraform

USAGE:
    lazytf [directory]

ARGS:
    directory    Path to terraform project (default: current directory)

FLAGS:
    -h, --help       Show this help
    -v, --version    Show version

KEYBOARD SHORTCUTS:
    Navigation:
        j/k, ↑/↓     Move up/down
        1-6           Jump to panel by number
        [ ]           Cycle through panels
        Tab           Switch between sidebar and detail pane
        Enter/Space   Select item / Focus detail pane
        g/G           Top/bottom of detail pane
        d/u           Page down/up in detail pane

    Terraform Commands:
        p             Run terraform plan
        a             Run terraform apply (plan → review → apply)
        i             Run terraform init
        v             Run terraform validate
        f/F           Run terraform fmt check / fix
        D             Run terraform destroy (plan → review → destroy)
        P             Show providers
        R             Recall last dismissed plan
        W             Multi-workspace plan (select → plan → apply across workspaces)

    Plan Review:
        y             Confirm apply/destroy
        esc/q         Dismiss (saves plan for R recall)
        n/N           Next/previous resource change
        f             Toggle focused single-resource view
        z             Toggle compact diff (collapse unchanged heredocs)
        c             Copy current resource diff to clipboard

    Multi-Workspace (W):
        j/k           Select workspace
        d/u           Page down/up output
        y             Apply selected workspace
        A             Apply ALL with changes (sequential)
        esc           Close / cancel

    Context Keys (panel-specific):
        e             Edit file / jump to resource declaration
        s             Refresh state show (Resources)
        s             Toggle skip-apply (Workspaces, persisted to .lazytf.yaml)
        t/u           Taint/untaint resource (Resources)
        x             Remove from state / delete workspace
        T             Targeted plan → apply (Resources, Modules)
        o             Open module directory (Modules)
        /             Filter workspaces (Workspaces)
        n             Create new workspace (Workspaces)

    Views:
        b             Switch to running command output (when browsing away)
        G             Dependency graph (from left pane)
        l             Toggle command log
        r             Refresh all data

    General:
        ?             Toggle help
        q/Ctrl+C      Quit`)
}

// cleanStalePlanFiles removes lazytf plan files in the temp directory that
// are older than 24 hours. These are orphaned from crashed sessions.
func cleanStalePlanFiles() {
	tmpDir := os.TempDir()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "lazytf-") && strings.HasSuffix(name, ".tfplan") {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(tmpDir, name))
			}
		}
	}
}

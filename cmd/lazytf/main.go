package main

import (
	"fmt"
	"os"
	"path/filepath"

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
        Tab           Switch between sidebar and main pane
        Enter/Space   Select item / Toggle section
        g/G           Go to top/bottom (main pane)
        d/u           Page down/up (main pane)

    Terraform Commands:
        p             Run terraform plan
        a             Run terraform apply (with confirmation)
        i             Run terraform init
        v             Run terraform validate
        f             Run terraform fmt -check
        F             Run terraform fmt (fix)
        d             Run terraform destroy (with confirmation)
        P             Show providers

    State Management:
        t             Taint selected resource
        u             Untaint selected resource

    Workspace & Vars:
        Enter         Switch workspace (when selected)
        Enter/Space   Toggle var-file selection

    General:
        r             Refresh data
        l             Toggle command log
        ?             Toggle help
        q/Ctrl+C      Quit`)
}

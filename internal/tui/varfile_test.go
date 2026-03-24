package tui

import (
	"testing"

	"github.com/cwar/lazytf/internal/terraform"
)

func TestMatchVarFileForWorkspace(t *testing.T) {
	m := &Model{
		files: []terraform.TfFile{
			{Name: "dev-gew4.tfvars", Path: "/proj/dev-gew4.tfvars", IsVars: true, Dir: ""},
			{Name: "prod-gew4.tfvars", Path: "/proj/prod-gew4.tfvars", IsVars: true, Dir: ""},
			{Name: "podcast-dev-gew4.tfvars", Path: "/proj/podcast-dev-gew4.tfvars", IsVars: true, Dir: ""},
			{Name: "main.tf", Path: "/proj/main.tf", IsVars: false, Dir: ""},
		},
	}

	tests := []struct {
		workspace string
		want      string
	}{
		{"dev-gew4", "/proj/dev-gew4.tfvars"},
		{"prod-gew4", "/proj/prod-gew4.tfvars"},
		{"podcast-dev-gew4", "/proj/podcast-dev-gew4.tfvars"},
		{"default", ""},
		{"", ""},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		got := m.matchVarFileForWorkspace(tt.workspace)
		if got != tt.want {
			t.Errorf("matchVarFileForWorkspace(%q) = %q, want %q", tt.workspace, got, tt.want)
		}
	}
}

func TestMatchVarFileForWorkspace_Priority(t *testing.T) {
	m := &Model{
		files: []terraform.TfFile{
			// subdir match
			{Name: "staging.tfvars", RelPath: "envs/staging.tfvars", Path: "/proj/envs/staging.tfvars", IsVars: true, Dir: "envs"},
			// auto match
			{Name: "staging.auto.tfvars", Path: "/proj/staging.auto.tfvars", IsVars: true, Dir: ""},
			// root exact match (highest priority)
			{Name: "staging.tfvars", Path: "/proj/staging.tfvars", IsVars: true, Dir: ""},
		},
	}

	got := m.matchVarFileForWorkspace("staging")
	if got != "/proj/staging.tfvars" {
		t.Errorf("expected root exact match, got %q", got)
	}
}

func TestMatchVarFileForWorkspace_AutoTfvars(t *testing.T) {
	m := &Model{
		files: []terraform.TfFile{
			{Name: "staging.auto.tfvars", Path: "/proj/staging.auto.tfvars", IsVars: true, Dir: ""},
		},
	}

	got := m.matchVarFileForWorkspace("staging")
	if got != "/proj/staging.auto.tfvars" {
		t.Errorf("expected auto.tfvars match, got %q", got)
	}
}

func TestMatchVarFileForWorkspace_SubdirFallback(t *testing.T) {
	m := &Model{
		files: []terraform.TfFile{
			{Name: "dev.tfvars", RelPath: "envs/dev.tfvars", Path: "/proj/envs/dev.tfvars", IsVars: true, Dir: "envs"},
		},
	}

	got := m.matchVarFileForWorkspace("dev")
	if got != "/proj/envs/dev.tfvars" {
		t.Errorf("expected subdir match, got %q", got)
	}
}

func TestAutoSelectVarFile_ManualOverride(t *testing.T) {
	m := &Model{
		workspace:   "dev-gew4",
		varFileManual: true,
		selectedVarFile: "/proj/custom.tfvars",
		files: []terraform.TfFile{
			{Name: "dev-gew4.tfvars", Path: "/proj/dev-gew4.tfvars", IsVars: true, Dir: ""},
		},
	}

	m.autoSelectVarFile()

	// Manual override should be preserved
	if m.selectedVarFile != "/proj/custom.tfvars" {
		t.Errorf("manual selection was overwritten: got %q", m.selectedVarFile)
	}
}

func TestAutoSelectVarFile_AutoMode(t *testing.T) {
	m := &Model{
		workspace:     "dev-gew4",
		varFileManual: false,
		files: []terraform.TfFile{
			{Name: "dev-gew4.tfvars", Path: "/proj/dev-gew4.tfvars", IsVars: true, Dir: ""},
			{Name: "prod-gew4.tfvars", Path: "/proj/prod-gew4.tfvars", IsVars: true, Dir: ""},
		},
	}

	m.autoSelectVarFile()

	if m.selectedVarFile != "/proj/dev-gew4.tfvars" {
		t.Errorf("expected auto-selection of dev-gew4.tfvars, got %q", m.selectedVarFile)
	}
}

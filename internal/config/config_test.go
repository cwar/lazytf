package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.IgnoreWorkspaces) != 0 {
		t.Errorf("expected empty ignore list, got %v", cfg.IgnoreWorkspaces)
	}
	if len(cfg.WorkspaceGroups) != 0 {
		t.Errorf("expected empty groups, got %v", cfg.WorkspaceGroups)
	}
}

func TestLoad_FullConfig(t *testing.T) {
	dir := t.TempDir()
	content := `
ignore_workspaces:
  - default
  - prod-gae2

workspace_groups:
  dev: dev-
  prod: prod-
  podcast: podcast
  osd: osd
`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.IgnoreWorkspaces) != 2 {
		t.Errorf("expected 2 ignored, got %d", len(cfg.IgnoreWorkspaces))
	}
	if cfg.IgnoreWorkspaces[0] != "default" {
		t.Errorf("expected 'default', got %q", cfg.IgnoreWorkspaces[0])
	}
	if len(cfg.WorkspaceGroups) != 4 {
		t.Errorf("expected 4 groups, got %d", len(cfg.WorkspaceGroups))
	}
	if cfg.WorkspaceGroups["dev"] != "dev-" {
		t.Errorf("expected dev group 'dev-', got %q", cfg.WorkspaceGroups["dev"])
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(":::invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestIsIgnored(t *testing.T) {
	cfg := Config{IgnoreWorkspaces: []string{"default", "prod-gae2"}}

	tests := []struct {
		name string
		want bool
	}{
		{"default", true},
		{"prod-gae2", true},
		{"Default", true},  // case-insensitive
		{"dev-gew4", false},
		{"prod-gew4", false},
	}
	for _, tt := range tests {
		if got := cfg.IsIgnored(tt.name); got != tt.want {
			t.Errorf("IsIgnored(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolveFilter(t *testing.T) {
	cfg := Config{WorkspaceGroups: map[string]string{
		"dev":     "dev-",
		"prod":    "prod-",
		"podcast": "podcast",
	}}

	tests := []struct {
		input string
		want  string
	}{
		{"dev", "dev-"},
		{"Dev", "dev-"},     // case-insensitive lookup
		{"prod", "prod-"},
		{"podcast", "podcast"},
		{"custom", "custom"}, // no match → pass through
		{"", ""},
	}
	for _, tt := range tests {
		if got := cfg.ResolveFilter(tt.input); got != tt.want {
			t.Errorf("ResolveFilter(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveFilter_NilGroups(t *testing.T) {
	cfg := Config{}
	if got := cfg.ResolveFilter("dev"); got != "dev" {
		t.Errorf("expected passthrough, got %q", got)
	}
}

func TestIsSkipApply(t *testing.T) {
	cfg := Config{SkipApplyWorkspaces: []string{"prod-gew4", "staging"}}

	tests := []struct {
		name string
		want bool
	}{
		{"prod-gew4", true},
		{"staging", true},
		{"Staging", true},   // case-insensitive
		{"dev-gew4", false},
		{"default", false},
	}
	for _, tt := range tests {
		if got := cfg.IsSkipApply(tt.name); got != tt.want {
			t.Errorf("IsSkipApply(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestToggleSkipApply_Add(t *testing.T) {
	cfg := Config{}
	cfg.ToggleSkipApply("prod-gew4")
	if !cfg.IsSkipApply("prod-gew4") {
		t.Error("expected prod-gew4 to be skip-apply after toggle")
	}
}

func TestToggleSkipApply_Remove(t *testing.T) {
	cfg := Config{SkipApplyWorkspaces: []string{"prod-gew4", "staging"}}
	cfg.ToggleSkipApply("prod-gew4")
	if cfg.IsSkipApply("prod-gew4") {
		t.Error("expected prod-gew4 to be removed from skip-apply after toggle")
	}
	if !cfg.IsSkipApply("staging") {
		t.Error("staging should remain in skip-apply")
	}
}

func TestToggleSkipApply_CaseInsensitive(t *testing.T) {
	cfg := Config{SkipApplyWorkspaces: []string{"Prod-Gew4"}}
	cfg.ToggleSkipApply("prod-gew4")
	if cfg.IsSkipApply("prod-gew4") {
		t.Error("case-insensitive toggle should have removed it")
	}
}

func TestSave_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		IgnoreWorkspaces:    []string{"default"},
		SkipApplyWorkspaces: []string{"prod-gew4"},
		WorkspaceGroups:     map[string]string{"dev": "dev-"},
	}
	if err := cfg.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Read it back
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(loaded.IgnoreWorkspaces) != 1 || loaded.IgnoreWorkspaces[0] != "default" {
		t.Errorf("IgnoreWorkspaces = %v, want [default]", loaded.IgnoreWorkspaces)
	}
	if len(loaded.SkipApplyWorkspaces) != 1 || loaded.SkipApplyWorkspaces[0] != "prod-gew4" {
		t.Errorf("SkipApplyWorkspaces = %v, want [prod-gew4]", loaded.SkipApplyWorkspaces)
	}
	if loaded.WorkspaceGroups["dev"] != "dev-" {
		t.Errorf("WorkspaceGroups = %v, want {dev: dev-}", loaded.WorkspaceGroups)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Write initial config
	initial := Config{
		IgnoreWorkspaces: []string{"default"},
		WorkspaceGroups:  map[string]string{"dev": "dev-", "prod": "prod-"},
	}
	if err := initial.Save(dir); err != nil {
		t.Fatal(err)
	}

	// Load, modify, save again
	cfg, _ := Load(dir)
	cfg.ToggleSkipApply("staging")
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}

	// Reload and verify all fields survived
	final, _ := Load(dir)
	if len(final.IgnoreWorkspaces) != 1 {
		t.Errorf("IgnoreWorkspaces lost: %v", final.IgnoreWorkspaces)
	}
	if len(final.WorkspaceGroups) != 2 {
		t.Errorf("WorkspaceGroups lost: %v", final.WorkspaceGroups)
	}
	if !final.IsSkipApply("staging") {
		t.Error("staging should be skip-apply after round-trip")
	}
}

func TestSave_EmptySkipApplyOmitted(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		IgnoreWorkspaces: []string{"default"},
	}
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}

	// Read raw file — should NOT contain skip_apply_workspaces
	data, _ := os.ReadFile(filepath.Join(dir, ConfigFileName))
	if strings.Contains(string(data), "skip_apply") {
		t.Errorf("empty skip_apply_workspaces should be omitted, got:\n%s", data)
	}
}

func TestLoad_WithSkipApply(t *testing.T) {
	dir := t.TempDir()
	content := `
ignore_workspaces:
  - default

skip_apply_workspaces:
  - prod-gew4
  - staging

workspace_groups:
  dev: dev-
`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.SkipApplyWorkspaces) != 2 {
		t.Errorf("expected 2 skip-apply, got %v", cfg.SkipApplyWorkspaces)
	}
	if !cfg.IsSkipApply("prod-gew4") {
		t.Error("expected prod-gew4 to be skip-apply")
	}
}

func TestFilterWorkspaces(t *testing.T) {
	cfg := Config{
		IgnoreWorkspaces: []string{"default", "prod-gae2"},
	}

	all := []string{
		"default", "dev-gew4", "dev-gae2", "prod-gew4", "prod-gae2",
		"podcast-dev", "podcast-prod", "osd-dev", "osd-prod",
	}

	// No filter: all minus ignored
	got := cfg.FilterWorkspaces(all, "")
	want := []string{"dev-gew4", "dev-gae2", "prod-gew4", "podcast-dev", "podcast-prod", "osd-dev", "osd-prod"}
	if len(got) != len(want) {
		t.Fatalf("FilterWorkspaces('') = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FilterWorkspaces('')[%d] = %q, want %q", i, got[i], w)
		}
	}

	// With filter
	got = cfg.FilterWorkspaces(all, "dev")
	want = []string{"dev-gew4", "dev-gae2", "podcast-dev", "osd-dev"}
	if len(got) != len(want) {
		t.Fatalf("FilterWorkspaces('dev') = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FilterWorkspaces('dev')[%d] = %q, want %q", i, got[i], w)
		}
	}

	// Exact prefix filter
	got = cfg.FilterWorkspaces(all, "dev-")
	want = []string{"dev-gew4", "dev-gae2"}
	if len(got) != len(want) {
		t.Fatalf("FilterWorkspaces('dev-') = %v, want %v", got, want)
	}

	// No match
	got = cfg.FilterWorkspaces(all, "staging")
	if len(got) != 0 {
		t.Errorf("FilterWorkspaces('staging') = %v, want empty", got)
	}
}

func TestFilterWorkspaces_ExcludesSkipApply(t *testing.T) {
	cfg := Config{
		IgnoreWorkspaces:    []string{"default"},
		SkipApplyWorkspaces: []string{"prod-gew4"},
	}
	all := []string{"default", "dev-gew4", "prod-gew4", "staging"}

	got := cfg.FilterWorkspaces(all, "")
	// default (ignored) and prod-gew4 (skip-apply) should both be excluded
	want := []string{"dev-gew4", "staging"}
	if len(got) != len(want) {
		t.Fatalf("FilterWorkspaces = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FilterWorkspaces[%d] = %q, want %q", i, got[i], w)
		}
	}
}

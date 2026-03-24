package terraform

import (
	"os"
	"testing"
)

func TestListFiles(t *testing.T) {
	// Test against babka-osd-infra if it exists
	dir := os.Getenv("TEST_TF_DIR")
	if dir == "" {
		dir = "/home/cwar/code/spotify/babka-osd-infra"
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("Test terraform directory not found")
	}

	r := NewRunner(dir)
	files, err := r.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles error: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("Expected at least one .tf file")
	}

	hasTf := false
	hasVars := false
	hasNested := false
	for _, f := range files {
		if !f.IsVars {
			hasTf = true
		}
		if f.IsVars {
			hasVars = true
		}
		if f.Depth > 0 {
			hasNested = true
		}
		t.Logf("Found: %-50s (dir=%-30s vars=%v depth=%d)", f.RelPath, f.Dir, f.IsVars, f.Depth)
	}

	if !hasTf {
		t.Error("Expected at least one .tf file")
	}
	if !hasVars {
		t.Error("Expected at least one .tfvars file")
	}
	if !hasNested {
		t.Error("Expected nested files in subdirectories (modules/)")
	}
	t.Logf("Total files found: %d", len(files))
}

func TestBuildFileTree(t *testing.T) {
	dir := os.Getenv("TEST_TF_DIR")
	if dir == "" {
		dir = "/home/cwar/code/spotify/babka-osd-infra"
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("Test terraform directory not found")
	}

	r := NewRunner(dir)
	files, err := r.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles error: %v", err)
	}

	tree := BuildFileTree(files)
	if tree == nil {
		t.Fatal("BuildFileTree returned nil")
	}

	if len(tree.Files) == 0 {
		t.Error("Expected root files")
	}
	t.Logf("Root files: %d, Root children dirs: %d", len(tree.Files), len(tree.Children))

	// Should have modules/ dir
	hasModules := false
	for _, child := range tree.Children {
		t.Logf("  Dir: %s (%d files, %d subdirs)", child.Name, len(child.Files), len(child.Children))
		if child.Name == "modules" {
			hasModules = true
		}
	}
	if !hasModules {
		t.Error("Expected 'modules' directory in tree")
	}
}

func TestParseResourceAddress(t *testing.T) {
	tests := []struct {
		addr     string
		wantType string
		wantName string
		wantMod  string
	}{
		{"aws_instance.web", "aws_instance", "web", ""},
		{"module.vpc.aws_subnet.private", "aws_subnet", "private", "vpc"},
		{"module.a.module.b.aws_s3_bucket.data", "aws_s3_bucket", "data", "a.b"},
		{"google_project.my_project", "google_project", "my_project", ""},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			r := parseResourceAddress(tt.addr)
			if r.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", r.Type, tt.wantType)
			}
			if r.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", r.Name, tt.wantName)
			}
			if r.Module != tt.wantMod {
				t.Errorf("Module = %q, want %q", r.Module, tt.wantMod)
			}
		})
	}
}

func TestNewRunnerDetection(t *testing.T) {
	r := NewRunner("/tmp")
	if r.Binary != "terraform" && r.Binary != "tofu" {
		t.Errorf("Binary = %q, want terraform or tofu", r.Binary)
	}
	t.Logf("Detected binary: %s", r.Binary)
}

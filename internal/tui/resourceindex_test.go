package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cwar/lazytf/internal/terraform"
)

func TestBuildResourceIndex(t *testing.T) {
	tmp := t.TempDir()

	// Write a sample .tf file
	content := `resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
}

data "aws_ami" "ubuntu" {
  most_recent = true
}

resource "aws_s3_bucket" "logs" {
  bucket = "my-logs"
}
`
	tfPath := filepath.Join(tmp, "main.tf")
	os.WriteFile(tfPath, []byte(content), 0644)

	files := []terraform.TfFile{
		{Name: "main.tf", Path: tfPath, RelPath: "main.tf", IsVars: false},
	}

	idx := buildResourceIndex(files, tmp)

	// Check expected entries
	tests := []struct {
		key      string
		wantLine int
	}{
		{"resource.aws_instance.web", 1},
		{"data.aws_ami.ubuntu", 6},
		{"resource.aws_s3_bucket.logs", 10},
	}
	for _, tc := range tests {
		loc, ok := idx[tc.key]
		if !ok {
			t.Errorf("missing index entry for %q", tc.key)
			continue
		}
		if loc.line != tc.wantLine {
			t.Errorf("index[%q].line = %d, want %d", tc.key, loc.line, tc.wantLine)
		}
		if loc.path != tfPath {
			t.Errorf("index[%q].path = %q, want %q", tc.key, loc.path, tfPath)
		}
	}

	// Verify non-existent key returns nothing
	if _, ok := idx["resource.aws_lambda.missing"]; ok {
		t.Error("should not have entry for non-existent resource")
	}
}

func TestBuildResourceIndex_SkipsVars(t *testing.T) {
	tmp := t.TempDir()

	content := `resource "aws_instance" "sneaky" {}`
	varsPath := filepath.Join(tmp, "terraform.tfvars")
	os.WriteFile(varsPath, []byte(content), 0644)

	files := []terraform.TfFile{
		{Name: "terraform.tfvars", Path: varsPath, RelPath: "terraform.tfvars", IsVars: true},
	}

	idx := buildResourceIndex(files, tmp)
	if len(idx) != 0 {
		t.Errorf("should skip .tfvars files, got %d entries", len(idx))
	}
}

func TestFindResourceFile_UsesIndex(t *testing.T) {
	m := Model{
		resourceIndex: map[string]resourceLocation{
			"resource.aws_instance.web":    {path: "/code/main.tf", line: 5},
			"data.aws_ami.ubuntu":          {path: "/code/data.tf", line: 10},
			"resource.aws_subnet.public":   {path: "/code/network.tf", line: 1},
		},
	}

	tests := []struct {
		address  string
		wantPath string
		wantLine int
	}{
		{"aws_instance.web", "/code/main.tf", 5},
		{"data.aws_ami.ubuntu", "/code/data.tf", 10},
		{"module.vpc.aws_subnet.public", "/code/network.tf", 1},
		{"module.a.module.b.aws_instance.web", "/code/main.tf", 5},
		{"aws_lambda.missing", "", 0},
	}

	for _, tc := range tests {
		path, line := m.findResourceFile(tc.address)
		if path != tc.wantPath || line != tc.wantLine {
			t.Errorf("findResourceFile(%q) = (%q, %d), want (%q, %d)",
				tc.address, path, line, tc.wantPath, tc.wantLine)
		}
	}
}

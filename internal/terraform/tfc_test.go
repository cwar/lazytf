package terraform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCloudBlock_Basic(t *testing.T) {
	source := `
terraform {
  cloud {
    organization = "babka"
    workspaces {
      tags = ["babka"]
    }
  }
}
`
	cc := parseCloudBlock(source)
	if cc == nil {
		t.Fatal("expected cloud config, got nil")
	}
	if cc.Organization != "babka" {
		t.Errorf("org = %q, want babka", cc.Organization)
	}
	if cc.Hostname != "app.terraform.io" {
		t.Errorf("hostname = %q, want app.terraform.io", cc.Hostname)
	}
	if len(cc.WorkspaceTags) != 1 || cc.WorkspaceTags[0] != "babka" {
		t.Errorf("tags = %v, want [babka]", cc.WorkspaceTags)
	}
}

func TestParseCloudBlock_CustomHostname(t *testing.T) {
	source := `
terraform {
  cloud {
    hostname     = "tfe.example.com"
    organization = "myorg"
    workspaces {
      name = "prod"
    }
  }
}
`
	cc := parseCloudBlock(source)
	if cc == nil {
		t.Fatal("expected cloud config, got nil")
	}
	if cc.Hostname != "tfe.example.com" {
		t.Errorf("hostname = %q", cc.Hostname)
	}
	if cc.WorkspaceName != "prod" {
		t.Errorf("workspace name = %q, want prod", cc.WorkspaceName)
	}
}

func TestParseCloudBlock_MultipleTags(t *testing.T) {
	source := `
terraform {
  cloud {
    organization = "acme"
    workspaces {
      tags = ["team-a", "prod"]
    }
  }
}
`
	cc := parseCloudBlock(source)
	if cc == nil {
		t.Fatal("expected cloud config")
	}
	if len(cc.WorkspaceTags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(cc.WorkspaceTags), cc.WorkspaceTags)
	}
	if cc.WorkspaceTags[0] != "team-a" || cc.WorkspaceTags[1] != "prod" {
		t.Errorf("tags = %v", cc.WorkspaceTags)
	}
}

func TestParseCloudBlock_NoCloudBlock(t *testing.T) {
	source := `
terraform {
  backend "s3" {
    bucket = "my-state"
    key    = "terraform.tfstate"
  }
}
`
	cc := parseCloudBlock(source)
	if cc != nil {
		t.Error("expected nil for non-cloud backend")
	}
}

func TestParseCloudBlock_NoTerraformBlock(t *testing.T) {
	source := `
resource "aws_instance" "web" {
  ami = "ami-123"
}
`
	cc := parseCloudBlock(source)
	if cc != nil {
		t.Error("expected nil when no terraform block")
	}
}

func TestParseCloudConfig_FromDir(t *testing.T) {
	dir := t.TempDir()

	// Write a backend.tf with cloud block
	err := os.WriteFile(filepath.Join(dir, "backend.tf"), []byte(`
terraform {
  cloud {
    organization = "babka"
    workspaces {
      tags = ["babka"]
    }
  }
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write another file without cloud block
	err = os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
resource "aws_instance" "web" {
  ami = "ami-123"
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cc := ParseCloudConfig(dir)
	if cc == nil {
		t.Fatal("expected cloud config from directory scan")
	}
	if cc.Organization != "babka" {
		t.Errorf("org = %q", cc.Organization)
	}
}

func TestParseCloudConfig_NoDirReturnsNil(t *testing.T) {
	cc := ParseCloudConfig("/nonexistent/path")
	if cc != nil {
		t.Error("expected nil for nonexistent dir")
	}
}

func TestParseListValue(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{`["babka"]`, []string{"babka"}},
		{`["a", "b", "c"]`, []string{"a", "b", "c"}},
		{`["single"]`, []string{"single"}},
		{`[]`, nil},
	}
	for _, tt := range tests {
		got := parseListValue(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseListValue(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseListValue(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestLoadTFCToken_FromCredFile(t *testing.T) {
	// Create a temporary credentials file
	dir := t.TempDir()
	credDir := filepath.Join(dir, ".terraform.d")
	os.MkdirAll(credDir, 0755)
	credFile := filepath.Join(credDir, "credentials.tfrc.json")
	err := os.WriteFile(credFile, []byte(`{
		"credentials": {
			"app.terraform.io": {
				"token": "test-token-12345"
			}
		}
	}`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Override HOME for this test
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	token := LoadTFCToken("app.terraform.io")
	if token != "test-token-12345" {
		t.Errorf("token = %q, want test-token-12345", token)
	}
}

func TestLoadTFCToken_MissingHost(t *testing.T) {
	dir := t.TempDir()
	credDir := filepath.Join(dir, ".terraform.d")
	os.MkdirAll(credDir, 0755)
	os.WriteFile(filepath.Join(credDir, "credentials.tfrc.json"), []byte(`{
		"credentials": {
			"app.terraform.io": {
				"token": "test-token"
			}
		}
	}`), 0600)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	token := LoadTFCToken("tfe.example.com")
	if token != "" {
		t.Errorf("expected empty token for unknown host, got %q", token)
	}
}

func TestLoadTFCToken_EnvVarTakesPrecedence(t *testing.T) {
	t.Setenv("TF_TOKEN_app_terraform_io", "env-token-override")

	token := LoadTFCToken("app.terraform.io")
	if token != "env-token-override" {
		t.Errorf("token = %q, want env-token-override", token)
	}
}

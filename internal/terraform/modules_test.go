package terraform

import (
	"os"
	"testing"
)

func TestParseModuleBlocks(t *testing.T) {
	source := `
module "druid" {
  source = "./modules/druid"

  cluster_name = var.cluster_name
  environment  = var.environment
  region       = var.region
}

module "gke" {
  source  = "terraform-google-modules/kubernetes-engine/google"
  version = "~> 27.0"

  project_id = var.project_id
}

resource "google_project" "main" {
  name = "test"
}

module "zookeeper" {
  source = "../shared/modules/zookeeper"

  replicas = 3
}
`

	mods := parseModuleBlocks(source, "main.tf")

	if len(mods) != 3 {
		t.Fatalf("Expected 3 modules, got %d", len(mods))
	}

	// Check druid
	if mods[0].Name != "druid" {
		t.Errorf("Module 0 name = %q, want 'druid'", mods[0].Name)
	}
	if mods[0].Source != "./modules/druid" {
		t.Errorf("Module 0 source = %q, want './modules/druid'", mods[0].Source)
	}
	if mods[0].SourceFile != "main.tf" {
		t.Errorf("Module 0 sourceFile = %q, want 'main.tf'", mods[0].SourceFile)
	}
	t.Logf("druid variables: %v", mods[0].Variables)

	// Check gke
	if mods[1].Name != "gke" {
		t.Errorf("Module 1 name = %q, want 'gke'", mods[1].Name)
	}
	if mods[1].Version != "~> 27.0" {
		t.Errorf("Module 1 version = %q, want '~> 27.0'", mods[1].Version)
	}
	t.Logf("gke display: %s", mods[1].ModuleSourceDisplay())

	// Check zookeeper
	if mods[2].Name != "zookeeper" {
		t.Errorf("Module 2 name = %q, want 'zookeeper'", mods[2].Name)
	}
}

func TestModuleSourceDisplay(t *testing.T) {
	tests := []struct {
		source  string
		version string
		want    string
	}{
		{"./modules/druid", "", "./modules/druid"},
		{"terraform-google-modules/kubernetes-engine/google", "~> 27.0", "kubernetes-engine ~> 27.0"},
		{"hashicorp/consul/aws", "", "consul"},
	}

	for _, tt := range tests {
		mod := ModuleCall{Source: tt.source, Version: tt.version}
		got := mod.ModuleSourceDisplay()
		if got != tt.want {
			t.Errorf("ModuleSourceDisplay(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestParseModulesFromRepo(t *testing.T) {
	dir := os.Getenv("TEST_TF_DIR")
	if dir == "" {
		dir = "/home/cwar/code/spotify/babka-osd-infra"
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("Test terraform directory not found")
	}

	r := NewRunner(dir)
	modules, err := r.ParseModules()
	if err != nil {
		t.Fatalf("ParseModules error: %v", err)
	}

	if len(modules) == 0 {
		t.Fatal("Expected at least one module call")
	}

	for _, m := range modules {
		t.Logf("Module: %-20s source=%-40s display=%-25s file=%s",
			m.Name, m.Source, m.ModuleSourceDisplay(), m.SourceFile)
	}
}

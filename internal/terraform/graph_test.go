package terraform

import (
	"strings"
	"testing"
)

func TestParseDOT(t *testing.T) {
	dot := `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] aws_instance.web (expand)" [label = "aws_instance.web"]
		"[root] aws_vpc.main (expand)" [label = "aws_vpc.main"]
		"[root] aws_subnet.private (expand)" [label = "aws_subnet.private"]
		"[root] module.druid (expand)" [label = "module.druid"]
		"[root] var.environment" [label = "var.environment"]
		"[root] provider[\"registry.terraform.io/hashicorp/aws\"]" [label = "provider.aws"]
		"[root] aws_instance.web (expand)" -> "[root] aws_subnet.private (expand)"
		"[root] aws_instance.web (expand)" -> "[root] provider[\"registry.terraform.io/hashicorp/aws\"]"
		"[root] aws_subnet.private (expand)" -> "[root] aws_vpc.main (expand)"
		"[root] module.druid (expand)" -> "[root] aws_vpc.main (expand)"
		"[root] module.druid (expand)" -> "[root] var.environment"
	}
}`

	g := ParseDOT(dot)

	if g == nil {
		t.Fatal("ParseDOT returned nil")
	}

	t.Logf("Nodes: %d", len(g.Nodes))
	for id, node := range g.Nodes {
		t.Logf("  %-50s type=%-10s label=%-30s deps=%d depby=%d",
			id, node.Type, node.Label, len(node.DepsOn), len(node.DepBy))
	}

	// Check we found the resources
	if len(g.Nodes) < 4 {
		t.Errorf("Expected at least 4 nodes, got %d", len(g.Nodes))
	}

	// Check edges
	webNode := g.Nodes["[root] aws_instance.web (expand)"]
	if webNode == nil {
		t.Fatal("Expected to find aws_instance.web node")
	}
	if len(webNode.DepsOn) != 2 {
		t.Errorf("aws_instance.web should depend on 2 nodes, got %d", len(webNode.DepsOn))
	}

	// Check label cleaning
	if webNode.Label != "aws_instance.web (expand)" {
		t.Logf("Label: %q (expected [root] prefix removed)", webNode.Label)
	}

	// Render
	t.Log("\n--- ASCII View ---")
	t.Log(g.RenderASCII())

	t.Log("\n--- Tree View ---")
	t.Log(g.RenderTree())

	t.Log("\n--- Summary ---")
	t.Log(g.Summary())
}

func TestParseDOTEmpty(t *testing.T) {
	g := ParseDOT("digraph { }")
	if g == nil {
		t.Fatal("ParseDOT returned nil for empty graph")
	}
	if len(g.Nodes) != 0 {
		t.Errorf("Expected 0 nodes for empty graph, got %d", len(g.Nodes))
	}

	tree := g.RenderTree()
	if !strings.Contains(tree, "empty") {
		t.Logf("Tree output: %q", tree)
	}
}

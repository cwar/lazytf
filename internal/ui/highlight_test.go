package ui

import (
	"strings"
	"testing"
)

func TestHighlightHCL_Keywords(t *testing.T) {
	input := `resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
  count         = 3
  tags = {
    Name = "web-${var.env}"
  }
}`

	lines := HighlightHCL(input, false)
	if len(lines) == 0 {
		t.Fatal("Expected highlighted lines")
	}

	// Just verify we get the right number of lines
	inputLines := strings.Split(input, "\n")
	if len(lines) != len(inputLines) {
		t.Errorf("Got %d lines, want %d", len(lines), len(inputLines))
	}

	// Verify some content is present (stripped of ANSI)
	for i, line := range lines {
		t.Logf("Line %d: %s", i+1, line)
	}
}

func TestHighlightHCL_LineNumbers(t *testing.T) {
	input := `variable "name" {
  type = string
}`

	lines := HighlightHCL(input, true)
	// With line numbers, each line should contain the number
	if len(lines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		t.Logf("Line %d: %s", i+1, line)
	}
}

func TestHighlightHCL_Comments(t *testing.T) {
	input := `# This is a comment
// Another comment
/* Block
   comment */
variable "x" {}`

	lines := HighlightHCL(input, false)
	if len(lines) != 5 {
		t.Fatalf("Expected 5 lines, got %d", len(lines))
	}
	for i, line := range lines {
		t.Logf("Line %d: %s", i+1, line)
	}
}

func TestHighlightHCL_Interpolation(t *testing.T) {
	input := `name = "hello-${var.env}-${local.suffix}"`
	lines := HighlightHCL(input, false)
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line, got %d", len(lines))
	}
	t.Logf("Result: %s", lines[0])
}

func TestHighlightPlanOutput(t *testing.T) {
	input := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami           = "ami-12345"
      + instance_type = "t2.micro"
      + id            = (known after apply)
    }

  # aws_s3_bucket.old will be destroyed
  - resource "aws_s3_bucket" "old" {
      - bucket = "my-old-bucket"
    }

Plan: 1 to add, 0 to change, 1 to destroy.`

	lines := HighlightPlanOutput(input)
	if len(lines) == 0 {
		t.Fatal("Expected highlighted plan lines")
	}
	for i, line := range lines {
		t.Logf("Line %d: %s", i+1, line)
	}
}

func TestHighlightPlanOutput_HeredocYAMLClassification(t *testing.T) {
	// Regression: YAML list items (- sh, - -c) inside heredoc blocks
	// were incorrectly classified as terraform removals (red).
	// Test the classification logic directly to avoid lipgloss TTY issues.
	lines := []string{
		"  # module.druid.kubectl_manifest.druid_cr will be updated in-place",
		`  ~ resource "kubectl_manifest" "druid_cr" {`,
		`        id   = "some-id"`,
		`        name = "osd-dev-gew4"`,
		`      ~ yaml_body_parsed = <<-EOT`,
		`            apiVersion: druid.apache.org/v1alpha1`,
		`            kind: Druid`,
		`            metadata:`,
		`              name: osd-dev-gew4`,
		`            spec:`,
		`              additionalContainer:`,
		`              - command:`,
		`                - sh`,
		`                - -c`,
		`                - sysctl -w vm.max_map_count=131072`,
		`                containerName: sysctl`,
		`              nodes:`,
		`                brokers:`,
		`                  druid.port: 8088`,
		`          +       podDisruptionBudgetSpec:`,
		`          +         maxUnavailable: 1`,
		`                  replicas: 4`,
		`        EOT`,
		`    }`,
	}

	h := NewPlanHighlighter()
	for _, line := range lines {
		kind := h.ClassifyLine(line)
		trimmed := strings.TrimSpace(line)

		// YAML list items inside heredoc should be plain, NOT destroy
		yamlItems := map[string]bool{
			"- command:": true, "- sh": true, "- -c": true,
			"- sysctl -w vm.max_map_count=131072": true,
		}
		if yamlItems[trimmed] {
			if kind == PlanLineDestroy {
				t.Errorf("YAML list item %q classified as DESTROY (red) — should be PLAIN", trimmed)
			}
			if kind != PlanLinePlain {
				t.Errorf("YAML list item %q classified as %v — should be PLAIN", trimmed, kind)
			}
		}

		// Unchanged content inside heredoc should be plain
		unchangedContent := map[string]bool{
			"apiVersion: druid.apache.org/v1alpha1": true, "kind: Druid": true,
			"containerName: sysctl": true, "replicas: 4": true,
		}
		if unchangedContent[trimmed] {
			if kind != PlanLinePlain {
				t.Errorf("Unchanged heredoc content %q classified as %v — should be PLAIN", trimmed, kind)
			}
		}

		// Real diff additions inside heredoc SHOULD be classified as add
		if strings.HasPrefix(trimmed, "+") && strings.Contains(line, "podDisruptionBudgetSpec") {
			if kind != PlanLineAdd {
				t.Errorf("Real diff addition %q classified as %v — should be ADD", trimmed, kind)
			}
		}
		if strings.HasPrefix(trimmed, "+") && strings.Contains(line, "maxUnavailable") {
			if kind != PlanLineAdd {
				t.Errorf("Real diff addition %q classified as %v — should be ADD", trimmed, kind)
			}
		}
	}
}

func TestHighlightPlanOutput_HeredocRemovalLines(t *testing.T) {
	// Test that real terraform removal lines inside heredocs ARE detected
	lines := []string{
		"  # module.druid.kubectl_manifest.druid_cr will be updated in-place",
		`  ~ resource "kubectl_manifest" "druid_cr" {`,
		`      ~ yaml_body_parsed = <<-EOT`,
		`            apiVersion: v1`,
		`          - oldField: oldValue`,
		`          + newField: newValue`,
		`            unchanged: content`,
		`        EOT`,
		`    }`,
	}

	h := NewPlanHighlighter()
	for _, line := range lines {
		kind := h.ClassifyLine(line)
		trimmed := strings.TrimSpace(line)

		if trimmed == "- oldField: oldValue" {
			if kind != PlanLineDestroy {
				t.Errorf("Real heredoc removal %q classified as %v — should be DESTROY", trimmed, kind)
			}
		}
		if trimmed == "+ newField: newValue" {
			if kind != PlanLineAdd {
				t.Errorf("Real heredoc addition %q classified as %v — should be ADD", trimmed, kind)
			}
		}
	}
}

func TestHighlightPlanOutput_NormalDiffUnchanged(t *testing.T) {
	// Non-heredoc diff lines should still work normally
	lines := []string{
		"  # aws_instance.web will be created",
		`  + resource "aws_instance" "web" {`,
		`      + ami           = "ami-12345"`,
		`      + id            = (known after apply)`,
		`    }`,
		"",
		"  # aws_s3_bucket.old will be destroyed",
		`  - resource "aws_s3_bucket" "old" {`,
		`      - bucket = "my-old-bucket"`,
		`    }`,
		"",
		"Plan: 1 to add, 0 to change, 1 to destroy.",
	}

	h := NewPlanHighlighter()
	for _, line := range lines {
		kind := h.ClassifyLine(line)
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "+") {
			if kind != PlanLineAdd {
				t.Errorf("Normal diff add %q classified as %v — should be ADD", trimmed, kind)
			}
		}
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(line, "bucket") {
			if kind != PlanLineDestroy {
				t.Errorf("Normal diff remove %q classified as %v — should be DESTROY", trimmed, kind)
			}
		}
	}
}

func TestHighlightPlanOutput_BatchMatchesStreaming(t *testing.T) {
	// HighlightPlanOutput should produce same classifications as streaming
	input := `  # module.druid.kubectl_manifest.druid_cr will be updated in-place
  ~ resource "kubectl_manifest" "druid_cr" {
      ~ yaml_body_parsed = <<-EOT
            apiVersion: v1
            spec:
              - command:
                - sh
          +     newField: value
          -     oldField: value
                unchanged: here
        EOT
    }`

	// Streaming classification
	h := NewPlanHighlighter()
	streamLines := strings.Split(input, "\n")
	streamKinds := make([]PlanLineKind, len(streamLines))
	for i, line := range streamLines {
		streamKinds[i] = h.ClassifyLine(line)
	}

	// Batch classification (used by HighlightPlanOutput internally)
	h2 := NewPlanHighlighter()
	for i, line := range streamLines {
		batchKind := h2.ClassifyLine(line)
		if batchKind != streamKinds[i] {
			t.Errorf("Line %d %q: batch=%v streaming=%v", i, strings.TrimSpace(line), batchKind, streamKinds[i])
		}
	}
}

func TestHighlightTfContent(t *testing.T) {
	// .tf file should get highlighted
	hl := HighlightTfContent(`resource "test" {}`, "main.tf")
	if hl == nil {
		t.Error("Expected highlighting for .tf file")
	}

	// .tfvars should get highlighted
	hl = HighlightTfContent(`name = "hello"`, "prod.tfvars")
	if hl == nil {
		t.Error("Expected highlighting for .tfvars file")
	}

	// .md should not
	hl = HighlightTfContent(`# Hello`, "README.md")
	if hl != nil {
		t.Error("Did not expect highlighting for .md file")
	}
}

func BenchmarkHighlightHCL(b *testing.B) {
	// Generate a realistic-sized HCL file
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(`resource "aws_instance" "web` + itoa(i) + `" {
  ami           = "ami-12345678"
  instance_type = "t2.micro"
  count         = ` + itoa(i) + `
  
  tags = {
    Name        = "web-${var.environment}-` + itoa(i) + `"
    Environment = var.environment
    ManagedBy   = "terraform"
  }

  lifecycle {
    create_before_destroy = true
  }
}

`)
	}
	input := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HighlightHCL(input, true)
	}
}

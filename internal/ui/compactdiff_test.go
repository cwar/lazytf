package ui

import (
	"strings"
	"testing"
)

func TestCompactDiff_CollapsesUnchangedHeredocLines(t *testing.T) {
	// A resource change with a huge heredoc and only a few changed lines.
	// CompactDiff should collapse unchanged runs to just context lines + a fold marker.
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
		`              namespace: druid`,
		`            spec:`,
		`              additionalContainer:`,
		`              - command:`,
		`                - sh`,
		`                - -c`,
		`                - sysctl -w vm.max_map_count=131072`,
		`                containerName: sysctl`,
		`                image: busybox`,
		`              nodes:`,
		`                brokers:`,
		`                  druid.port: 8088`,
		`          +       podDisruptionBudgetSpec:`,
		`          +         maxUnavailable: 1`,
		`                  replicas: 4`,
		`                  resources:`,
		`                    requests:`,
		`                      cpu: 15`,
		`                      memory: 51Gi`,
		`                hot-tier:`,
		`                  druid.port: 8088`,
		`                  replicas: 8`,
		`        EOT`,
		`    }`,
	}

	result := CompactDiff(lines, 3) // 3 lines of context
	if len(result) == 0 {
		t.Fatal("Expected non-empty compact output")
	}

	// Should be much shorter than the original
	if len(result) >= len(lines) {
		t.Errorf("Compact diff should reduce line count: got %d, original %d", len(result), len(lines))
	}

	// The actual diff lines (+) MUST be present
	foundPDB := false
	foundMaxUnavail := false
	for _, l := range result {
		if strings.Contains(l, "podDisruptionBudgetSpec") {
			foundPDB = true
		}
		if strings.Contains(l, "maxUnavailable") {
			foundMaxUnavail = true
		}
	}
	if !foundPDB {
		t.Error("Compact diff missing + podDisruptionBudgetSpec line")
	}
	if !foundMaxUnavail {
		t.Error("Compact diff missing + maxUnavailable line")
	}

	// Resource header should still be present
	foundHeader := false
	for _, l := range result {
		if strings.Contains(l, "will be updated") {
			foundHeader = true
		}
	}
	if !foundHeader {
		t.Error("Compact diff missing resource header line")
	}

	// There should be at least one fold marker for the collapsed sections
	foundFold := false
	for _, l := range result {
		if strings.Contains(l, "···") || strings.Contains(l, "...") || strings.Contains(l, "lines") {
			foundFold = true
		}
	}
	if !foundFold {
		t.Error("Compact diff should contain a fold marker for collapsed lines")
	}

	// Log the output for visual inspection
	for i, l := range result {
		t.Logf("Line %2d: %s", i, l)
	}
}

func TestCompactDiff_PreservesNonHeredocDiffs(t *testing.T) {
	// Normal (non-heredoc) resource changes should be preserved fully
	lines := []string{
		"  # aws_instance.web will be updated in-place",
		`  ~ resource "aws_instance" "web" {`,
		`      ~ instance_type = "t2.micro" -> "t2.large"`,
		`        id            = "i-12345"`,
		`    }`,
	}

	result := CompactDiff(lines, 3)

	// For a short resource, all lines should be kept (nothing to collapse)
	if len(result) != len(lines) {
		t.Errorf("Short resource should not be compacted: got %d, want %d", len(result), len(lines))
	}
}

func TestCompactDiff_MultipleChangesInHeredoc(t *testing.T) {
	// Multiple changed regions within a heredoc should each get context
	lines := []string{
		"  # module.x.resource will be updated in-place",
		`  ~ resource "kubectl_manifest" "cr" {`,
		`      ~ yaml_body_parsed = <<-EOT`,
		`            line1: unchanged`,
		`            line2: unchanged`,
		`            line3: unchanged`,
		`            line4: unchanged`,
		`            line5: unchanged`,
		`          + added_near_top: value`,
		`            line6: unchanged`,
		`            line7: unchanged`,
		`            line8: unchanged`,
		`            line9: unchanged`,
		`            line10: unchanged`,
		`            line11: unchanged`,
		`            line12: unchanged`,
		`            line13: unchanged`,
		`          - removed_near_bottom: old`,
		`          + replaced_near_bottom: new`,
		`            line14: unchanged`,
		`            line15: unchanged`,
		`        EOT`,
		`    }`,
	}

	result := CompactDiff(lines, 2) // 2 lines of context

	// Both changes must be present
	foundAdded := false
	foundRemoved := false
	foundReplaced := false
	for _, l := range result {
		if strings.Contains(l, "added_near_top") {
			foundAdded = true
		}
		if strings.Contains(l, "removed_near_bottom") {
			foundRemoved = true
		}
		if strings.Contains(l, "replaced_near_bottom") {
			foundReplaced = true
		}
	}
	if !foundAdded {
		t.Error("Missing added_near_top")
	}
	if !foundRemoved {
		t.Error("Missing removed_near_bottom")
	}
	if !foundReplaced {
		t.Error("Missing replaced_near_bottom")
	}

	// Should still be shorter than original
	if len(result) >= len(lines) {
		t.Errorf("Multi-change compact should reduce line count: got %d, original %d", len(result), len(lines))
	}

	for i, l := range result {
		t.Logf("Line %2d: %s", i, l)
	}
}

func TestCompactDiff_EmptyInput(t *testing.T) {
	result := CompactDiff(nil, 3)
	if len(result) != 0 {
		t.Errorf("Empty input should produce empty output, got %d lines", len(result))
	}

	result = CompactDiff([]string{}, 3)
	if len(result) != 0 {
		t.Errorf("Empty slice should produce empty output, got %d lines", len(result))
	}
}

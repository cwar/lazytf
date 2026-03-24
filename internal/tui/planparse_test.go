package tui

import (
	"strings"
	"testing"
)

func TestParsePlanChanges(t *testing.T) {
	plan := `
Terraform used the selected providers to generate the following execution plan.

  # module.druid.google_sql_database.druid will be updated in-place
  ~ resource "google_sql_database" "druid" {
        name     = "druid"
      ~ charset  = "UTF8" -> "utf8"
    }

  # google_compute_instance.web will be created
  + resource "google_compute_instance" "web" {
      + name = "web-server"
    }

  # aws_iam_role.old will be destroyed
  - resource "aws_iam_role" "old" {
      - name = "old-role"
    }

  # aws_instance.app must be replaced
  -/+ resource "aws_instance" "app" {
      ~ ami = "ami-old" -> "ami-new"
    }

Plan: 1 to add, 1 to change, 1 to destroy.
`

	lines := strings.Split(plan, "\n")
	changes := parsePlanChanges(lines)

	if len(changes) != 4 {
		t.Fatalf("expected 4 changes, got %d", len(changes))
	}

	tests := []struct {
		addr   string
		action string
	}{
		{"module.druid.google_sql_database.druid", "update"},
		{"google_compute_instance.web", "create"},
		{"aws_iam_role.old", "destroy"},
		{"aws_instance.app", "replace"},
	}

	for i, tt := range tests {
		if changes[i].Address != tt.addr {
			t.Errorf("change[%d].Address = %q, want %q", i, changes[i].Address, tt.addr)
		}
		if changes[i].Action != tt.action {
			t.Errorf("change[%d].Action = %q, want %q", i, changes[i].Action, tt.action)
		}
		if changes[i].Line == 0 {
			t.Errorf("change[%d].Line should not be 0", i)
		}
		if changes[i].EndLine <= changes[i].Line {
			t.Errorf("change[%d].EndLine (%d) should be > Line (%d)", i, changes[i].EndLine, changes[i].Line)
		}
	}

	// Verify blocks don't overlap
	for i := 1; i < len(changes); i++ {
		if changes[i].Line < changes[i-1].EndLine {
			t.Errorf("change[%d].Line (%d) overlaps with change[%d].EndLine (%d)",
				i, changes[i].Line, i-1, changes[i-1].EndLine)
		}
	}
}

func TestParsePlanChanges_NoChanges(t *testing.T) {
	plan := `No changes. Infrastructure is up-to-date.`
	lines := strings.Split(plan, "\n")
	changes := parsePlanChanges(lines)

	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestParsePlanChanges_DataSource(t *testing.T) {
	plan := `  # data.google_client_config.default will be read during apply`
	lines := strings.Split(plan, "\n")
	changes := parsePlanChanges(lines)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Action != "read" {
		t.Errorf("expected action 'read', got %q", changes[0].Action)
	}
}

func TestActionIcon(t *testing.T) {
	tests := map[string]string{
		"create":  "+",
		"destroy": "-",
		"update":  "~",
		"replace": "±",
		"read":    "?",
		"change":  "•",
	}
	for action, want := range tests {
		got := actionIcon(action)
		if got != want {
			t.Errorf("actionIcon(%q) = %q, want %q", action, got, want)
		}
	}
}

package tui

import "strings"

// planChange represents a single resource change block in a terraform plan.
type planChange struct {
	Address string // e.g. "module.druid.google_sql_database.druid"
	Action  string // "create", "update", "destroy", "replace", "read"
	Line    int    // line index in the plan output where this block starts
	EndLine int    // line index where this block ends (exclusive)
}

// parsePlanChanges extracts resource change blocks from terraform plan output lines.
// Each change starts with a line like:
//
//	# module.foo.aws_instance.bar will be updated in-place
//	# aws_s3_bucket.logs will be created
//	# aws_iam_role.old will be destroyed
//	# aws_instance.web must be replaced
//
// EndLine is set to the start of the next block (or end of output), trimming
// trailing blank lines so focused view is compact.
func parsePlanChanges(lines []string) []planChange {
	var changes []planChange

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}

		rest := trimmed[2:] // strip "# "

		// Look for "will be" or "must be" action patterns
		action := ""
		address := ""

		if idx := strings.Index(rest, " will be "); idx > 0 {
			address = rest[:idx]
			actionPart := rest[idx+9:] // after " will be "
			action = classifyAction(actionPart)
		} else if idx := strings.Index(rest, " must be "); idx > 0 {
			address = rest[:idx]
			actionPart := rest[idx+9:] // after " must be "
			action = classifyAction(actionPart)
		} else if idx := strings.Index(rest, " has been deleted"); idx > 0 {
			address = rest[:idx]
			action = "delete"
		} else if idx := strings.Index(rest, " will be read during apply"); idx > 0 {
			address = rest[:idx]
			action = "read"
		} else {
			continue
		}

		// Sanity check: address should look like a terraform resource
		if address == "" || !looksLikeAddress(address) {
			continue
		}

		changes = append(changes, planChange{
			Address: address,
			Action:  action,
			Line:    i,
		})
	}

	// Fill in EndLine for each change: runs until the next change starts,
	// with trailing blank lines trimmed for a cleaner focused view.
	for i := range changes {
		var rawEnd int
		if i+1 < len(changes) {
			rawEnd = changes[i+1].Line
		} else {
			rawEnd = len(lines)
		}
		// Trim trailing blank lines
		end := rawEnd
		for end > changes[i].Line && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
		changes[i].EndLine = end
	}

	return changes
}

func classifyAction(s string) string {
	switch {
	case strings.HasPrefix(s, "created"):
		return "create"
	case strings.HasPrefix(s, "updated"):
		return "update"
	case strings.HasPrefix(s, "destroyed"):
		return "destroy"
	case strings.HasPrefix(s, "replaced"):
		return "replace"
	case strings.HasPrefix(s, "read"):
		return "read"
	default:
		return "change"
	}
}

// looksLikeAddress does a quick check that the string looks like a terraform
// resource address (contains a dot and no spaces).
func looksLikeAddress(s string) bool {
	return strings.Contains(s, ".") && !strings.Contains(s, " ")
}

// actionIcon returns an icon character for a plan action.
func actionIcon(action string) string {
	switch action {
	case "create":
		return "+"
	case "destroy", "delete":
		return "-"
	case "update":
		return "~"
	case "replace":
		return "±"
	case "read":
		return "?"
	default:
		return "•"
	}
}

// actionLabel returns a short human-readable label for a plan action.
func actionLabel(action string) string {
	switch action {
	case "create":
		return "create "
	case "destroy":
		return "destroy "
	case "delete":
		return "delete "
	case "update":
		return "update "
	case "replace":
		return "replace "
	case "read":
		return "read "
	default:
		return ""
	}
}

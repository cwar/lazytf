package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// HCL syntax highlighting styles
var (
	HclKeyword = lipgloss.NewStyle().
			Foreground(Magenta).
			Bold(true)

	HclType = lipgloss.NewStyle().
		Foreground(Cyan).
		Bold(true)

	HclString = lipgloss.NewStyle().
			Foreground(Green)

	HclNumber = lipgloss.NewStyle().
			Foreground(Orange)

	HclComment = lipgloss.NewStyle().
			Foreground(DimGray).
			Italic(true)

	HclAttribute = lipgloss.NewStyle().
			Foreground(Blue)

	HclBrace = lipgloss.NewStyle().
			Foreground(Yellow)

	HclBool = lipgloss.NewStyle().
			Foreground(Orange).
			Bold(true)

	HclFunction = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BB86FC"))

	HclInterp = lipgloss.NewStyle().
			Foreground(Cyan)

	HclOperator = lipgloss.NewStyle().
			Foreground(Magenta)

	HclLabel = lipgloss.NewStyle().
			Foreground(White).
			Bold(true)

	HclLineNum = lipgloss.NewStyle().
			Foreground(DimGray).
			Width(4).
			Align(lipgloss.Right)

	HclLineNumSep = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#333333"))
)

// HCL top-level block keywords
var hclBlockKeywords = map[string]bool{
	"resource":   true,
	"data":       true,
	"variable":   true,
	"output":     true,
	"locals":     true,
	"module":     true,
	"provider":   true,
	"terraform":  true,
	"moved":      true,
	"import":     true,
	"check":      true,
	"removed":    true,
}

// HCL meta-argument / nested block keywords
var hclMetaKeywords = map[string]bool{
	"for_each":         true,
	"count":            true,
	"depends_on":       true,
	"lifecycle":        true,
	"provisioner":      true,
	"connection":       true,
	"dynamic":          true,
	"content":          true,
	"required_providers": true,
	"required_version": true,
	"backend":          true,
	"cloud":            true,
}

// HCL built-in functions (common ones)
var hclFunctions = map[string]bool{
	"abs": true, "ceil": true, "floor": true, "log": true, "max": true,
	"min": true, "pow": true, "signum": true, "chomp": true, "format": true,
	"formatlist": true, "indent": true, "join": true, "lower": true,
	"regex": true, "regexall": true, "replace": true, "split": true,
	"strrev": true, "substr": true, "title": true, "trim": true,
	"trimprefix": true, "trimsuffix": true, "trimspace": true, "upper": true,
	"alltrue": true, "anytrue": true, "chunklist": true, "coalesce": true,
	"coalescelist": true, "compact": true, "concat": true, "contains": true,
	"distinct": true, "element": true, "flatten": true, "index": true,
	"keys": true, "length": true, "list": true, "lookup": true, "map": true,
	"matchkeys": true, "merge": true, "one": true, "range": true,
	"reverse": true, "setintersection": true, "setproduct": true,
	"setsubtract": true, "setunion": true, "slice": true, "sort": true,
	"sum": true, "transpose": true, "values": true, "zipmap": true,
	"can": true, "nonsensitive": true, "sensitive": true, "tobool": true,
	"tolist": true, "tomap": true, "tonumber": true, "toset": true,
	"tostring": true, "try": true, "type": true,
	"file": true, "fileexists": true, "fileset": true, "filebase64": true,
	"templatefile": true, "base64decode": true, "base64encode": true,
	"base64gzip": true, "csvdecode": true, "jsondecode": true,
	"jsonencode": true, "textdecodebase64": true, "textencodebase64": true,
	"urlencode": true, "yamldecode": true, "yamlencode": true,
	"abspath": true, "dirname": true, "pathexpand": true, "basename": true,
	"cidrhost": true, "cidrnetmask": true, "cidrsubnet": true,
	"cidrsubnets": true, "md5": true, "rsadecrypt": true, "sha1": true,
	"sha256": true, "sha512": true, "uuid": true, "uuidv5": true,
	"bcrypt": true, "filesha1": true, "filesha256": true, "filesha512": true,
	"filemd5": true, "filebase64sha256": true, "filebase64sha512": true,
	"formatdate": true, "timeadd": true, "timecmp": true, "timestamp": true,
	"plantimestamp": true, "endswith": true, "startswith": true,
}

// HCL type keywords
var hclTypes = map[string]bool{
	"string": true, "number": true, "bool": true, "list": true,
	"map": true, "set": true, "object": true, "tuple": true,
	"any": true, "optional": true,
}

// HighlightHCL highlights an HCL source string and returns
// a slice of highlighted lines ready for rendering.
func HighlightHCL(source string, showLineNumbers bool) []string {
	rawLines := strings.Split(source, "\n")
	highlighted := make([]string, len(rawLines))
	inBlockComment := false

	for i, line := range rawLines {
		var prefix string
		if showLineNumbers {
			num := HclLineNum.Render(strings.Repeat(" ", 4-numWidth(i+1)) + itoa(i+1))
			sep := HclLineNumSep.Render(" │ ")
			prefix = num + sep
		}

		if inBlockComment {
			endIdx := strings.Index(line, "*/")
			if endIdx >= 0 {
				inBlockComment = false
				before := line[:endIdx+2]
				after := line[endIdx+2:]
				highlighted[i] = prefix + HclComment.Render(before) + highlightHCLLine(after)
			} else {
				highlighted[i] = prefix + HclComment.Render(line)
			}
			continue
		}

		// Check for block comment start
		if idx := strings.Index(line, "/*"); idx >= 0 {
			endIdx := strings.Index(line[idx+2:], "*/")
			if endIdx >= 0 {
				// Block comment on single line
				before := line[:idx]
				comment := line[idx : idx+2+endIdx+2]
				after := line[idx+2+endIdx+2:]
				highlighted[i] = prefix + highlightHCLLine(before) + HclComment.Render(comment) + highlightHCLLine(after)
			} else {
				inBlockComment = true
				before := line[:idx]
				comment := line[idx:]
				highlighted[i] = prefix + highlightHCLLine(before) + HclComment.Render(comment)
			}
			continue
		}

		highlighted[i] = prefix + highlightHCLLine(line)
	}

	return highlighted
}

// highlightHCLLine highlights a single line of HCL (no block comment handling).
func highlightHCLLine(line string) string {
	// Handle line comments first
	commentIdx := findLineComment(line)
	var comment string
	if commentIdx >= 0 {
		comment = line[commentIdx:]
		line = line[:commentIdx]
	}

	result := highlightHCLTokens(line)

	if comment != "" {
		result += HclComment.Render(comment)
	}

	return result
}

// findLineComment finds the start of a line comment (# or //) outside of strings.
func findLineComment(line string) int {
	inString := false
	escape := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if !inString {
			if ch == '#' {
				return i
			}
			if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
				return i
			}
		}
	}
	return -1
}

// highlightHCLTokens tokenizes and highlights a line of HCL.
func highlightHCLTokens(line string) string {
	if len(line) == 0 {
		return ""
	}

	var result strings.Builder
	i := 0

	for i < len(line) {
		ch := line[i]

		// Whitespace
		if ch == ' ' || ch == '\t' {
			result.WriteByte(ch)
			i++
			continue
		}

		// Strings (double-quoted) — with interpolation support
		if ch == '"' {
			str, advance := consumeString(line[i:])
			result.WriteString(highlightString(str))
			i += advance
			continue
		}

		// Heredoc
		if ch == '<' && i+1 < len(line) && line[i+1] == '<' {
			// Just render rest of line as string
			result.WriteString(HclString.Render(line[i:]))
			break
		}

		// Braces and brackets
		if ch == '{' || ch == '}' || ch == '[' || ch == ']' || ch == '(' || ch == ')' {
			result.WriteString(HclBrace.Render(string(ch)))
			i++
			continue
		}

		// Operators
		if ch == '=' || ch == '!' || ch == '<' || ch == '>' || ch == '?' || ch == ':' {
			op := string(ch)
			if i+1 < len(line) && (line[i+1] == '=' || line[i+1] == '>') {
				op += string(line[i+1])
				i++
			}
			result.WriteString(HclOperator.Render(op))
			i++
			continue
		}

		// Comma, dot
		if ch == ',' || ch == '.' {
			result.WriteByte(ch)
			i++
			continue
		}

		// Numbers
		if ch >= '0' && ch <= '9' {
			start := i
			for i < len(line) && (line[i] >= '0' && line[i] <= '9' || line[i] == '.' || line[i] == 'e' || line[i] == 'E' || line[i] == '+' || line[i] == '-' || line[i] == 'x' || line[i] == 'X' || (line[i] >= 'a' && line[i] <= 'f') || (line[i] >= 'A' && line[i] <= 'F')) {
				i++
			}
			result.WriteString(HclNumber.Render(line[start:i]))
			continue
		}

		// Identifiers (words)
		if isIdentStart(ch) {
			start := i
			for i < len(line) && isIdentChar(line[i]) {
				i++
			}
			word := line[start:i]

			// Check what comes after (skip whitespace)
			rest := line[i:]
			restTrimmed := strings.TrimLeft(rest, " \t")

			switch {
			case word == "true" || word == "false" || word == "null":
				result.WriteString(HclBool.Render(word))
			case hclBlockKeywords[word]:
				result.WriteString(HclKeyword.Render(word))
			case hclMetaKeywords[word]:
				result.WriteString(HclKeyword.Render(word))
			case hclTypes[word]:
				result.WriteString(HclType.Render(word))
			case hclFunctions[word] && len(restTrimmed) > 0 && restTrimmed[0] == '(':
				result.WriteString(HclFunction.Render(word))
			case word == "var" || word == "local" || word == "each" || word == "self" || word == "count":
				result.WriteString(HclKeyword.Render(word))
			case word == "for" || word == "in" || word == "if" || word == "else" || word == "endif":
				result.WriteString(HclKeyword.Render(word))
			case len(restTrimmed) > 0 && restTrimmed[0] == '=':
				// attribute = value
				result.WriteString(HclAttribute.Render(word))
			case len(restTrimmed) > 0 && restTrimmed[0] == '{':
				// block_name {  — nested block
				result.WriteString(HclKeyword.Render(word))
			case len(restTrimmed) > 0 && restTrimmed[0] == '"':
				// label "name" { — this is a block with label
				result.WriteString(HclKeyword.Render(word))
			default:
				result.WriteString(word)
			}
			continue
		}

		// Anything else
		result.WriteByte(ch)
		i++
	}

	return result.String()
}

// consumeString consumes a double-quoted string from the start of s,
// handling escape sequences. Returns the string and how many bytes consumed.
func consumeString(s string) (string, int) {
	if len(s) == 0 || s[0] != '"' {
		return "", 0
	}
	i := 1
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == '"' {
			return s[:i+1], i + 1
		}
		i++
	}
	// Unterminated string
	return s, len(s)
}

// highlightString highlights a string literal, including ${...} interpolations.
func highlightString(s string) string {
	if len(s) < 2 {
		return HclString.Render(s)
	}

	var result strings.Builder
	i := 0

	for i < len(s) {
		// Look for interpolation: ${ or %{
		if i+1 < len(s) && (s[i] == '$' || s[i] == '%') && s[i+1] == '{' {
			// Find matching close brace (simple, doesn't handle nested)
			depth := 0
			start := i
			i += 2
			depth = 1
			for i < len(s) && depth > 0 {
				if s[i] == '{' {
					depth++
				} else if s[i] == '}' {
					depth--
				}
				i++
			}
			interp := s[start:i]
			result.WriteString(HclInterp.Render(interp))
			continue
		}
		// Regular string character
		result.WriteString(HclString.Render(string(s[i])))
		i++
	}

	return result.String()
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9') || ch == '-'
}

func numWidth(n int) int {
	if n < 10 {
		return 1
	}
	if n < 100 {
		return 2
	}
	if n < 1000 {
		return 3
	}
	if n < 10000 {
		return 4
	}
	return 5
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// HighlightTfContent highlights terraform file content and returns
// pre-rendered lines. If the file is not a .tf/.tfvars file it
// returns nil so the caller can fall back to plain text.
func HighlightTfContent(content string, filename string) []string {
	ext := ""
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			ext = filename[i:]
			break
		}
	}

	switch ext {
	case ".tf", ".tfvars":
		return HighlightHCL(content, true)
	default:
		return nil
	}
}

// PlanLineKind classifies how a plan output line should be styled.
type PlanLineKind int

const (
	PlanLinePlain          PlanLineKind = iota // no special styling
	PlanLineAdd                               // + addition (green)
	PlanLineDestroy                           // - removal (red)
	PlanLineChange                            // ~ change (yellow)
	PlanLineHeaderCreate                      // # ... will be created
	PlanLineHeaderDestroy                     // # ... will be destroyed
	PlanLineHeaderChange                      // # ... will be updated/replaced
	PlanLineHeaderInfo                        // # ... other
	PlanLineKnownAfter                        // (known after apply)
	PlanLineForceReplace                      // forces replacement
	PlanLineSummary                           // Plan: X to add ...
	PlanLineNoChanges                         // No changes
	PlanLineSeparator                         // ─── or ---
	PlanLineError                             // Error
	PlanLineWarning                           // Warning
	PlanLineBoilerplate                       // Terraform will perform ...
)

// PlanHighlighter tracks state for context-aware plan output highlighting.
// It distinguishes real terraform diff markers (+/-/~) from content that
// happens to start with those characters (e.g. YAML list items inside heredocs).
type PlanHighlighter struct {
	inDiff        bool
	inHeredoc     bool
	heredocMarker string
	heredocIndent int // base content indent inside heredoc (-1 = unknown)
}

// NewPlanHighlighter creates a new stateful plan highlighter.
func NewPlanHighlighter() *PlanHighlighter {
	return &PlanHighlighter{heredocIndent: -1}
}

// ClassifyLine determines how a plan output line should be styled,
// updating internal state to track heredoc blocks.
func (h *PlanHighlighter) ClassifyLine(line string) PlanLineKind {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	indent := len(line) - len(trimmed)

	// Check for heredoc end BEFORE processing (the EOT line itself is plain)
	if h.inHeredoc && h.heredocMarker != "" && trimmed == h.heredocMarker {
		h.inHeredoc = false
		h.heredocMarker = ""
		h.heredocIndent = -1
		return PlanLinePlain
	}

	// Inside heredoc: use indent-aware diff detection
	if h.inHeredoc {
		// Establish base indent from the first non-diff, non-empty content line
		if h.heredocIndent < 0 && trimmed != "" {
			ch := trimmed[0]
			if ch != '+' && ch != '-' && ch != '~' {
				h.heredocIndent = indent
			}
		}

		// Classify +/-/~ inside heredoc
		if trimmed != "" && (trimmed[0] == '+' || trimmed[0] == '-' || trimmed[0] == '~') {
			if h.heredocIndent >= 0 && indent < h.heredocIndent {
				// The marker is LEFT of the content base indent → real diff marker
				switch trimmed[0] {
				case '+':
					return PlanLineAdd
				case '-':
					return PlanLineDestroy
				case '~':
					return PlanLineChange
				}
			}
			// At or deeper than base indent → this is content (e.g. YAML list -)
		}

		return PlanLinePlain
	}

	// Check for heredoc start on this line (after classifying it)
	// We defer entering heredoc mode until after processing the current line.
	defer func() {
		if h.inDiff && !h.inHeredoc {
			if marker := extractHeredocMarker(line); marker != "" {
				h.inHeredoc = true
				h.heredocMarker = marker
				h.heredocIndent = -1
			}
		}
	}()

	// --- Normal (non-heredoc) classification ---

	switch {
	// Resource header lines
	case strings.HasPrefix(trimmed, "# ") && (strings.Contains(trimmed, "will be") || strings.Contains(trimmed, "must be")):
		h.inDiff = true
		if strings.Contains(trimmed, "created") {
			return PlanLineHeaderCreate
		} else if strings.Contains(trimmed, "destroyed") {
			return PlanLineHeaderDestroy
		} else if strings.Contains(trimmed, "updated") || strings.Contains(trimmed, "replaced") {
			return PlanLineHeaderChange
		}
		return PlanLineHeaderInfo

	// Diff markers (outside heredoc)
	case h.inDiff && strings.HasPrefix(trimmed, "+"):
		return PlanLineAdd
	case h.inDiff && strings.HasPrefix(trimmed, "-"):
		return PlanLineDestroy
	case h.inDiff && strings.HasPrefix(trimmed, "~"):
		return PlanLineChange

	case strings.Contains(line, "(known after apply)"):
		return PlanLineKnownAfter
	case strings.Contains(line, "forces replacement"):
		return PlanLineForceReplace
	case strings.HasPrefix(trimmed, "Plan:"):
		return PlanLineSummary
	case strings.Contains(line, "No changes") || strings.Contains(line, "Infrastructure is up-to-date"):
		return PlanLineNoChanges
	case strings.HasPrefix(trimmed, "───") || strings.HasPrefix(trimmed, "---"):
		h.inDiff = false
		return PlanLineSeparator
	case strings.HasPrefix(trimmed, "Error"):
		return PlanLineError
	case strings.HasPrefix(trimmed, "Warning"):
		return PlanLineWarning
	case strings.Contains(line, "Terraform will perform") || strings.Contains(line, "Terraform used the selected"):
		return PlanLineBoilerplate
	}

	return PlanLinePlain
}

// HighlightLine classifies and highlights a single terraform plan output line.
// Call sequentially for streaming output — tracks heredoc state across calls.
func (h *PlanHighlighter) HighlightLine(line string) string {
	return applyPlanLineStyle(line, h.ClassifyLine(line))
}

// applyPlanLineStyle renders a line with the appropriate style for its kind.
func applyPlanLineStyle(line string, kind PlanLineKind) string {
	switch kind {
	case PlanLineAdd:
		return PlanAdd.Render(line)
	case PlanLineDestroy:
		return PlanDestroy.Render(line)
	case PlanLineChange:
		return PlanChange.Render(line)
	case PlanLineHeaderCreate:
		return PlanAdd.Bold(true).Render(line)
	case PlanLineHeaderDestroy:
		return PlanDestroy.Bold(true).Render(line)
	case PlanLineHeaderChange:
		return PlanChange.Bold(true).Render(line)
	case PlanLineHeaderInfo:
		return PlanInfo.Render(line)
	case PlanLineKnownAfter:
		return HclComment.Render(line)
	case PlanLineForceReplace:
		return PlanDestroy.Bold(true).Render(line)
	case PlanLineSummary:
		return renderPlanSummary(line)
	case PlanLineNoChanges:
		return SuccessStyle.Render(line)
	case PlanLineSeparator:
		return lipgloss.NewStyle().Foreground(DimGray).Render(line)
	case PlanLineError:
		return ErrorStyle.Render(line)
	case PlanLineWarning:
		return WarningStyle.Render(line)
	case PlanLineBoilerplate:
		return lipgloss.NewStyle().Foreground(MediumGray).Render(line)
	default:
		return line
	}
}

// extractHeredocMarker returns the heredoc end marker if line contains
// a heredoc start (<<- or <<), otherwise returns "".
func extractHeredocMarker(line string) string {
	// Look for <<- first (indented heredoc), then <<
	for _, prefix := range []string{"<<-", "<<"} {
		idx := strings.Index(line, prefix)
		if idx < 0 {
			continue
		}
		after := strings.TrimSpace(line[idx+len(prefix):])
		// The marker is the first word (letters, digits, underscore)
		var marker strings.Builder
		for _, ch := range after {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
				marker.WriteRune(ch)
			} else {
				break
			}
		}
		if marker.Len() > 0 {
			return marker.String()
		}
	}
	return ""
}

// HighlightPlanOutput highlights terraform plan output with richer
// analysis than the basic HighlightPlanLine. Uses PlanHighlighter
// internally for heredoc-aware diff detection.
func HighlightPlanOutput(output string) []string {
	lines := strings.Split(output, "\n")
	result := make([]string, len(lines))
	h := NewPlanHighlighter()

	for i, line := range lines {
		result[i] = h.HighlightLine(line)
	}

	return result
}

// renderPlanSummary colorizes the "Plan: X to add, Y to change, Z to destroy" line.
func renderPlanSummary(line string) string {
	var result strings.Builder
	result.WriteString(PlanInfo.Bold(true).Render("Plan: "))

	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(line), "Plan: "), ", ")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case strings.Contains(part, "add"):
			result.WriteString(PlanAdd.Bold(true).Render(part))
		case strings.Contains(part, "change"):
			result.WriteString(PlanChange.Bold(true).Render(part))
		case strings.Contains(part, "destroy"):
			result.WriteString(PlanDestroy.Bold(true).Render(part))
		default:
			result.WriteString(part)
		}
		if i < len(parts)-1 {
			result.WriteString(lipgloss.NewStyle().Foreground(MediumGray).Render(", "))
		}
	}

	return result.String()
}

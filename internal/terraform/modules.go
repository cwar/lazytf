package terraform

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModuleCall represents a module block found in .tf files.
type ModuleCall struct {
	Name       string // module label, e.g. "druid"
	Source     string // source attribute value
	SourceFile string // which .tf file defines it (relative)
	Version    string // version constraint (if any)
	Variables  []string // input variable names passed
}

// ParseModules scans all .tf files for module blocks and extracts metadata.
func (r *Runner) ParseModules() ([]ModuleCall, error) {
	files, err := r.ListFiles()
	if err != nil {
		return nil, err
	}

	var modules []ModuleCall
	for _, f := range files {
		if f.IsVars {
			continue
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		mods := parseModuleBlocks(string(data), f.RelPath)
		modules = append(modules, mods...)
	}

	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Name < modules[j].Name
	})
	return modules, nil
}

// parseModuleBlocks extracts module calls from HCL source text.
// This is a lightweight regex-free parser that handles the common patterns.
func parseModuleBlocks(source, relPath string) []ModuleCall {
	var modules []ModuleCall
	lines := strings.Split(source, "\n")

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Look for: module "name" {
		if strings.HasPrefix(line, "module ") {
			name := extractLabel(line, "module")
			if name != "" {
				mod := ModuleCall{
					Name:       name,
					SourceFile: relPath,
				}
				// Scan the block for source, version, variables
				depth := 0
				if strings.Contains(line, "{") {
					depth = 1
				}
				i++
				for i < len(lines) && depth > 0 {
					bline := strings.TrimSpace(lines[i])

					// Track brace depth
					depth += strings.Count(bline, "{") - strings.Count(bline, "}")

					// Extract attributes at depth 1
					if depth >= 1 {
						if attr, val := parseAttribute(bline); attr != "" {
							switch attr {
							case "source":
								mod.Source = val
							case "version":
								mod.Version = val
							default:
								// Track variable bindings
								mod.Variables = append(mod.Variables, attr)
							}
						}
					}
					i++
				}
				modules = append(modules, mod)
				continue
			}
		}
		i++
	}
	return modules
}

// extractLabel extracts the quoted label from a block declaration.
// e.g. `resource "aws_instance" "web" {` → "aws_instance" (for keyword "resource")
// e.g. `module "druid" {` → "druid" (for keyword "module")
func extractLabel(line, keyword string) string {
	rest := strings.TrimPrefix(line, keyword)
	rest = strings.TrimSpace(rest)

	// Find first quoted string
	start := strings.IndexByte(rest, '"')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(rest[start+1:], '"')
	if end < 0 {
		return ""
	}
	return rest[start+1 : start+1+end]
}

// parseAttribute parses a simple `key = "value"` or `key = value` line.
func parseAttribute(line string) (string, string) {
	// Skip comments
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		return "", ""
	}
	// Skip blocks (keyword "label" {)
	if strings.Contains(line, "{") {
		return "", ""
	}

	eqIdx := strings.IndexByte(line, '=')
	if eqIdx < 0 {
		return "", ""
	}
	// Make sure it's not == or => 
	if eqIdx+1 < len(line) && (line[eqIdx+1] == '=' || line[eqIdx+1] == '>') {
		return "", ""
	}
	if eqIdx > 0 && (line[eqIdx-1] == '!' || line[eqIdx-1] == '<' || line[eqIdx-1] == '>') {
		return "", ""
	}

	key := strings.TrimSpace(line[:eqIdx])
	val := strings.TrimSpace(line[eqIdx+1:])

	// Strip quotes from value
	val = strings.Trim(val, "\"")

	return key, val
}

// ModuleSourceDisplay returns a display-friendly version of the source.
func (m ModuleCall) ModuleSourceDisplay() string {
	if m.Source == "" {
		return "(unknown)"
	}
	// Local paths
	if strings.HasPrefix(m.Source, "./") || strings.HasPrefix(m.Source, "../") {
		return m.Source
	}
	// Registry modules: namespace/name/provider
	parts := strings.Split(m.Source, "/")
	if len(parts) == 3 {
		display := parts[1]
		if m.Version != "" {
			display += " " + m.Version
		}
		return display
	}
	// Git URLs, S3, etc.
	if strings.Contains(m.Source, "github.com") || strings.Contains(m.Source, "ghe.") {
		// Extract repo name
		idx := strings.LastIndex(m.Source, "/")
		if idx >= 0 {
			repo := m.Source[idx+1:]
			repo = strings.TrimSuffix(repo, ".git")
			// Handle ?ref=xxx
			if qIdx := strings.Index(repo, "?"); qIdx >= 0 {
				ref := ""
				after := repo[qIdx+1:]
				if strings.HasPrefix(after, "ref=") {
					ref = strings.TrimPrefix(after, "ref=")
				}
				repo = repo[:qIdx]
				if ref != "" {
					return repo + "@" + ref
				}
			}
			return repo
		}
	}
	return m.Source
}

// ModuleDir returns the directory where the local module lives,
// resolved relative to the working directory.
func (m ModuleCall) ModuleDir(workDir string) string {
	if !strings.HasPrefix(m.Source, "./") && !strings.HasPrefix(m.Source, "../") {
		return ""
	}
	// Resolve relative to the file that defines the module
	baseDir := filepath.Dir(m.SourceFile)
	resolved := filepath.Join(workDir, baseDir, m.Source)
	resolved = filepath.Clean(resolved)

	rel, err := filepath.Rel(workDir, resolved)
	if err != nil {
		return m.Source
	}
	return rel
}

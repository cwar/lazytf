package terraform

import (
	"sort"
	"strconv"
	"strings"
)

// GraphNode represents a node in the dependency graph.
type GraphNode struct {
	ID       string   // raw node ID from DOT
	Label    string   // cleaned display label
	Type     string   // "resource", "module", "var", "output", "provider", "data", "local"
	DepsOn   []string // nodes this depends on (outgoing edges)
	DepBy    []string // nodes that depend on this (incoming edges)
}

// Graph represents a parsed terraform dependency graph.
type Graph struct {
	Nodes map[string]*GraphNode
	Roots []string // nodes with no incoming dependencies
}

// ParseDOT parses terraform's DOT graph output into a Graph structure.
func ParseDOT(dot string) *Graph {
	g := &Graph{
		Nodes: make(map[string]*GraphNode),
	}

	lines := strings.Split(dot, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip non-edge lines
		if !strings.Contains(line, "->") {
			// But check for standalone node declarations
			if strings.HasPrefix(line, "\"") {
				nodeID := extractDotNode(line)
				if nodeID != "" {
					g.ensureNode(nodeID)
				}
			}
			continue
		}

		// Parse edge: "node_a" -> "node_b"
		parts := strings.SplitN(line, "->", 2)
		if len(parts) != 2 {
			continue
		}
		from := extractDotNode(parts[0])
		to := extractDotNode(parts[1])
		if from == "" || to == "" {
			continue
		}

		fromNode := g.ensureNode(from)
		toNode := g.ensureNode(to)

		fromNode.DepsOn = append(fromNode.DepsOn, to)
		toNode.DepBy = append(toNode.DepBy, from)
	}

	// Find roots (no dependencies)
	for id, node := range g.Nodes {
		if len(node.DepsOn) == 0 {
			g.Roots = append(g.Roots, id)
		}
	}
	sort.Strings(g.Roots)

	return g
}

func (g *Graph) ensureNode(id string) *GraphNode {
	if n, ok := g.Nodes[id]; ok {
		return n
	}
	n := &GraphNode{
		ID:    id,
		Label: cleanNodeLabel(id),
		Type:  classifyNode(id),
	}
	g.Nodes[id] = n
	return n
}

// extractDotNode extracts a node ID from a DOT fragment like `"[root] aws_instance.web"`.
// Also handles `"[root] aws_instance.web" [label = "..."]` by stripping attributes.
func extractDotNode(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ";")
	s = strings.TrimSpace(s)

	if len(s) == 0 {
		return ""
	}

	// Extract just the first quoted string.
	// DOT uses escaped quotes inside: \"registry...\"
	// So we find the closing " that isn't preceded by \
	if s[0] == '"' {
		for i := 1; i < len(s); i++ {
			if s[i] == '"' && (i < 2 || s[i-1] != '\\') {
				return s[1:i]
			}
		}
	}

	// Unquoted — strip any [attr = ...] suffix
	if idx := strings.Index(s, " ["); idx >= 0 {
		s = s[:idx]
	}

	return strings.TrimSpace(s)
}

// cleanNodeLabel removes the "[root] " prefix and other DOT cruft.
func cleanNodeLabel(id string) string {
	label := id
	label = strings.TrimPrefix(label, "[root] ")
	// Remove "provider[\"registry.terraform.io/..." → "provider.aws"
	if strings.HasPrefix(label, "provider[") {
		inner := strings.TrimPrefix(label, "provider[")
		inner = strings.TrimSuffix(inner, "]")
		inner = strings.Trim(inner, "\"\\")
		// Extract just the provider name
		parts := strings.Split(inner, "/")
		if len(parts) > 0 {
			label = "provider." + parts[len(parts)-1]
		}
	}
	return label
}

// classifyNode determines the type of a graph node from its ID.
func classifyNode(id string) string {
	clean := strings.TrimPrefix(id, "[root] ")
	switch {
	case strings.HasPrefix(clean, "module."):
		return "module"
	case strings.HasPrefix(clean, "data."):
		return "data"
	case strings.HasPrefix(clean, "var."):
		return "var"
	case strings.HasPrefix(clean, "output."):
		return "output"
	case strings.HasPrefix(clean, "local."):
		return "local"
	case strings.HasPrefix(clean, "provider"):
		return "provider"
	case clean == "root":
		return "root"
	default:
		return "resource"
	}
}

// RenderASCII renders the graph as an ASCII dependency tree.
// Shows each resource and what it depends on.
func (g *Graph) RenderASCII() string {
	var sb strings.Builder

	// Collect interesting nodes (skip meta nodes)
	var interesting []*GraphNode
	for _, node := range g.Nodes {
		switch node.Type {
		case "resource", "module", "data":
			interesting = append(interesting, node)
		}
	}
	sort.Slice(interesting, func(i, j int) bool {
		return interesting[i].Label < interesting[j].Label
	})

	if len(interesting) == 0 {
		sb.WriteString("  (no resources in graph)\n")
		return sb.String()
	}

	for _, node := range interesting {
		// Node header with icon
		icon := resourceIcon(node.Type)
		sb.WriteString(icon + " " + node.Label + "\n")

		// Show dependencies
		deps := filterDeps(g, node.DepsOn)
		for i, dep := range deps {
			prefix := "├── "
			if i == len(deps)-1 {
				prefix = "└── "
			}
			depNode := g.Nodes[dep]
			if depNode != nil {
				depIcon := resourceIcon(depNode.Type)
				sb.WriteString("  " + prefix + depIcon + " " + depNode.Label + "\n")
			}
		}

		// Show what depends on this
		depBy := filterDeps(g, node.DepBy)
		if len(depBy) > 0 {
			sb.WriteString("  ⮤  needed by: ")
			var names []string
			for _, d := range depBy {
				if dn := g.Nodes[d]; dn != nil {
					names = append(names, dn.Label)
				}
			}
			sb.WriteString(strings.Join(names, ", ") + "\n")
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// RenderTree renders the graph as a top-down dependency tree from roots.
func (g *Graph) RenderTree() string {
	var sb strings.Builder
	visited := make(map[string]bool)

	// Start from nodes that nothing depends on (top-level)
	var topLevel []string
	for id, node := range g.Nodes {
		if len(node.DepBy) == 0 && node.Type != "root" && node.Type != "provider" {
			topLevel = append(topLevel, id)
		}
	}
	sort.Strings(topLevel)

	if len(topLevel) == 0 {
		// Fallback: use roots
		topLevel = g.Roots
	}

	for _, id := range topLevel {
		node := g.Nodes[id]
		if node == nil || node.Type == "root" || node.Type == "provider" {
			continue
		}
		g.renderTreeNode(&sb, id, "", true, visited, 0)
	}

	if sb.Len() == 0 {
		sb.WriteString("  (empty graph)\n")
	}

	return sb.String()
}

func (g *Graph) renderTreeNode(sb *strings.Builder, id, prefix string, isLast bool, visited map[string]bool, depth int) {
	if depth > 10 {
		return // prevent infinite recursion
	}
	node := g.Nodes[id]
	if node == nil {
		return
	}

	// Connector
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if depth == 0 {
		connector = ""
	}

	icon := resourceIcon(node.Type)
	label := node.Label

	if visited[id] {
		sb.WriteString(prefix + connector + icon + " " + label + " (↻)\n")
		return
	}
	visited[id] = true

	sb.WriteString(prefix + connector + icon + " " + label + "\n")

	// Get interesting children (what this depends on)
	children := filterDeps(g, node.DepsOn)

	childPrefix := prefix
	if depth > 0 {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, childID := range children {
		g.renderTreeNode(sb, childID, childPrefix, i == len(children)-1, visited, depth+1)
	}
}

// filterDeps returns only interesting dependency IDs (resources, modules, data).
func filterDeps(g *Graph, deps []string) []string {
	var filtered []string
	for _, d := range deps {
		node := g.Nodes[d]
		if node == nil {
			continue
		}
		switch node.Type {
		case "resource", "module", "data":
			filtered = append(filtered, d)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i] < filtered[j]
	})
	return filtered
}

func resourceIcon(nodeType string) string {
	switch nodeType {
	case "resource":
		return "◆"
	case "module":
		return "📦"
	case "data":
		return "◇"
	case "var":
		return "⊳"
	case "output":
		return "⊲"
	case "provider":
		return "⚙"
	case "local":
		return "∟"
	default:
		return "·"
	}
}

// Summary returns a one-line summary of the graph.
func (g *Graph) Summary() string {
	counts := map[string]int{}
	for _, node := range g.Nodes {
		counts[node.Type]++
	}

	var parts []string
	for _, t := range []string{"resource", "data", "module", "var", "output"} {
		if c := counts[t]; c > 0 {
			parts = append(parts, titleCase(t)+": "+strconv.Itoa(c))
		}
	}
	edges := 0
	for _, node := range g.Nodes {
		edges += len(node.DepsOn)
	}
	parts = append(parts, "Edges: "+strconv.Itoa(edges))
	return strings.Join(parts, "  │  ")
}

// titleCase capitalises the first byte of an ASCII string.
func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

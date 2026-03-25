// Package config handles lazytf configuration from .lazytf.yaml files.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ConfigFileName = ".lazytf.yaml"

// Config represents the lazytf configuration.
type Config struct {
	// IgnoreWorkspaces lists workspace names to exclude from multi-workspace operations.
	// These workspaces still appear in the normal workspace panel but are skipped
	// when running batch plan/apply across all workspaces.
	IgnoreWorkspaces []string `yaml:"ignore_workspaces,omitempty"`

	// SkipApplyWorkspaces lists workspace names that are planned but skipped
	// during "apply all" (A key). Unlike IgnoreWorkspaces which excludes them
	// entirely, these workspaces ARE planned so you can review their changes,
	// but they won't be included in batch apply unless you un-skip them (x key)
	// or apply them individually (y key).
	SkipApplyWorkspaces []string `yaml:"skip_apply_workspaces,omitempty"`

	// WorkspaceGroups maps a short name to a substring filter for quick selection.
	// Example: {"dev": "dev", "prod": "prod", "podcast": "podcast"}
	// When the user types a group name, it's expanded to the filter substring.
	WorkspaceGroups map[string]string `yaml:"workspace_groups,omitempty"`
}

// Load reads the config file from the given directory.
// Returns a zero Config (not an error) if the file doesn't exist.
func Load(dir string) (Config, error) {
	path := filepath.Join(dir, ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// IsIgnored returns true if the workspace name is in the ignore list.
func (c *Config) IsIgnored(workspace string) bool {
	for _, ign := range c.IgnoreWorkspaces {
		if strings.EqualFold(ign, workspace) {
			return true
		}
	}
	return false
}

// IsSkipApply returns true if the workspace is in the skip-apply list.
func (c *Config) IsSkipApply(workspace string) bool {
	for _, s := range c.SkipApplyWorkspaces {
		if strings.EqualFold(s, workspace) {
			return true
		}
	}
	return false
}

// ToggleSkipApply adds or removes a workspace from the skip-apply list.
func (c *Config) ToggleSkipApply(workspace string) {
	for i, s := range c.SkipApplyWorkspaces {
		if strings.EqualFold(s, workspace) {
			c.SkipApplyWorkspaces = append(c.SkipApplyWorkspaces[:i], c.SkipApplyWorkspaces[i+1:]...)
			return
		}
	}
	c.SkipApplyWorkspaces = append(c.SkipApplyWorkspaces, workspace)
}

// Save writes the config to .lazytf.yaml in the given directory.
func (c *Config) Save(dir string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFileName), data, 0644)
}

// ResolveFilter expands a filter string: if it matches a workspace group name,
// returns the group's filter value. Otherwise returns the input unchanged.
// This lets users type "dev" and have it resolve to whatever the group defines.
func (c *Config) ResolveFilter(input string) string {
	if c.WorkspaceGroups == nil {
		return input
	}
	if expanded, ok := c.WorkspaceGroups[strings.ToLower(input)]; ok {
		return expanded
	}
	return input
}

// FilterWorkspaces returns workspaces that are not ignored/skipped and match
// the given substring filter (case-insensitive). Empty filter matches all
// non-excluded workspaces. Both ignore_workspaces and skip_apply_workspaces
// are excluded.
func (c *Config) FilterWorkspaces(workspaces []string, filter string) []string {
	filter = strings.ToLower(filter)
	var result []string
	for _, ws := range workspaces {
		if c.IsIgnored(ws) || c.IsSkipApply(ws) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(ws), filter) {
			continue
		}
		result = append(result, ws)
	}
	return result
}

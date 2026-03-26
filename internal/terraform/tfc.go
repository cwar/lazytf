package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CloudConfig represents the terraform cloud block configuration.
type CloudConfig struct {
	Organization string // organization name
	Hostname     string // API hostname (default: app.terraform.io)
	WorkspaceTags []string // workspace tag filters
	WorkspaceName string   // explicit workspace name (if set)
}

// TFCRun represents a single run from Terraform Cloud.
type TFCRun struct {
	ID         string
	Status     string // e.g. "planned", "applied", "errored", "planned_and_finished"
	Message    string // commit message or trigger reason
	CreatedAt  time.Time
	IsDestroy  bool
	HasChanges bool
	PlanID     string // relationships.plan.data.id
	Source     string // e.g. "tfe-ui", "tfe-api", "tfe-cli"
}

// ParseCloudConfig extracts the cloud block from .tf files in the given directory.
// Returns nil if no cloud block is found (e.g., local backend).
func ParseCloudConfig(workDir string) *CloudConfig {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workDir, e.Name()))
		if err != nil {
			continue
		}
		if cc := parseCloudBlock(string(data)); cc != nil {
			return cc
		}
	}
	return nil
}

// parseCloudBlock extracts cloud config from HCL source text.
func parseCloudBlock(source string) *CloudConfig {
	lines := strings.Split(source, "\n")

	// Find "terraform {" block, then "cloud {" inside it
	inTerraform := false
	inCloud := false
	inWorkspaces := false
	depth := 0
	cloudDepth := 0
	wsDepth := 0

	cc := &CloudConfig{
		Hostname: "app.terraform.io",
	}
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inTerraform {
			if strings.HasPrefix(trimmed, "terraform ") && strings.Contains(trimmed, "{") {
				inTerraform = true
				depth = 1
			}
			continue
		}

		// Track brace depth within terraform block
		opens := strings.Count(trimmed, "{")
		closes := strings.Count(trimmed, "}")

		if !inCloud {
			if strings.HasPrefix(trimmed, "cloud ") && strings.Contains(trimmed, "{") {
				inCloud = true
				cloudDepth = depth + 1
				found = true
				depth += opens - closes
				continue
			}
		}

		if inCloud && !inWorkspaces {
			if strings.HasPrefix(trimmed, "workspaces ") && strings.Contains(trimmed, "{") {
				inWorkspaces = true
				wsDepth = depth + 1
				depth += opens - closes
				continue
			}
		}

		depth += opens - closes

		if inWorkspaces {
			if attr, val := parseAttribute(trimmed); attr != "" {
				switch attr {
				case "tags":
					cc.WorkspaceTags = parseListValue(val)
				case "name":
					cc.WorkspaceName = val
				}
			}
			if depth < wsDepth {
				inWorkspaces = false
			}
		} else if inCloud {
			if attr, val := parseAttribute(trimmed); attr != "" {
				switch attr {
				case "organization":
					cc.Organization = val
				case "hostname":
					cc.Hostname = val
				}
			}
			if depth < cloudDepth {
				inCloud = false
			}
		}

		if depth <= 0 {
			inTerraform = false
		}
	}

	if !found {
		return nil
	}
	return cc
}

// parseListValue extracts string values from an HCL list like ["a", "b"].
func parseListValue(val string) []string {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	var result []string
	for _, part := range strings.Split(val, ",") {
		s := strings.TrimSpace(part)
		s = strings.Trim(s, "\"")
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// ─── TFC API Token ───────────────────────────────────────

// tfcCredentials represents the structure of ~/.terraform.d/credentials.tfrc.json
type tfcCredentials struct {
	Credentials map[string]struct {
		Token string `json:"token"`
	} `json:"credentials"`
}

// LoadTFCToken reads the API token for the given hostname from the
// terraform credentials file. Checks TF_TOKEN_* env var first.
func LoadTFCToken(hostname string) string {
	// Check environment variable first: TF_TOKEN_app_terraform_io
	envKey := "TF_TOKEN_" + strings.ReplaceAll(strings.ReplaceAll(hostname, ".", "_"), "-", "__")
	if token := os.Getenv(envKey); token != "" {
		return token
	}

	// Read credentials file
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".terraform.d", "credentials.tfrc.json"))
	if err != nil {
		return ""
	}

	var creds tfcCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return ""
	}

	if entry, ok := creds.Credentials[hostname]; ok {
		return entry.Token
	}
	return ""
}

// ─── TFC API Client ─────────────────────────────────────

// TFCClient interacts with the Terraform Cloud API.
type TFCClient struct {
	Hostname string
	Token    string
	Org      string
	client   *http.Client
}

// NewTFCClient creates a TFC API client from a CloudConfig.
func NewTFCClient(cc *CloudConfig) *TFCClient {
	token := LoadTFCToken(cc.Hostname)
	return &TFCClient{
		Hostname: cc.Hostname,
		Token:    token,
		Org:      cc.Organization,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// HasToken returns true if the client has a valid API token.
func (c *TFCClient) HasToken() bool {
	return c.Token != ""
}

// ListRuns fetches recent runs for a workspace from TFC.
func (c *TFCClient) ListRuns(ctx context.Context, workspaceName string, limit int) ([]TFCRun, error) {
	// First, resolve workspace name → workspace ID
	wsID, err := c.getWorkspaceID(ctx, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("workspace lookup: %w", err)
	}

	url := fmt.Sprintf("https://%s/api/v2/workspaces/%s/runs?page[size]=%d", c.Hostname, wsID, limit)
	body, err := c.apiGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	var resp struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Status    string `json:"status"`
				Message   string `json:"message"`
				CreatedAt string `json:"created-at"`
				IsDestroy bool   `json:"is-destroy"`
				HasChanges bool  `json:"has-changes"`
				Source    string `json:"source"`
			} `json:"attributes"`
			Relationships struct {
				Plan struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"plan"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse runs: %w", err)
	}

	var runs []TFCRun
	for _, d := range resp.Data {
		t, _ := time.Parse(time.RFC3339, d.Attributes.CreatedAt)
		runs = append(runs, TFCRun{
			ID:         d.ID,
			Status:     d.Attributes.Status,
			Message:    d.Attributes.Message,
			CreatedAt:  t,
			IsDestroy:  d.Attributes.IsDestroy,
			HasChanges: d.Attributes.HasChanges,
			PlanID:     d.Relationships.Plan.Data.ID,
			Source:     d.Attributes.Source,
		})
	}
	return runs, nil
}

// GetPlanLog fetches the raw plan log output for a given plan ID.
func (c *TFCClient) GetPlanLog(ctx context.Context, planID string) (string, error) {
	// Get plan details to find the log-read-url
	url := fmt.Sprintf("https://%s/api/v2/plans/%s", c.Hostname, planID)
	body, err := c.apiGet(ctx, url)
	if err != nil {
		return "", fmt.Errorf("get plan: %w", err)
	}

	var resp struct {
		Data struct {
			Attributes struct {
				LogReadURL string `json:"log-read-url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse plan: %w", err)
	}

	logURL := resp.Data.Attributes.LogReadURL
	if logURL == "" {
		return "", fmt.Errorf("no log URL available")
	}

	// Fetch the actual log (no auth needed — the URL is pre-signed)
	logReq, err := http.NewRequestWithContext(ctx, "GET", logURL, nil)
	if err != nil {
		return "", err
	}
	logResp, err := c.client.Do(logReq)
	if err != nil {
		return "", fmt.Errorf("fetch log: %w", err)
	}
	defer logResp.Body.Close()

	logBody, err := io.ReadAll(logResp.Body)
	if err != nil {
		return "", fmt.Errorf("read log: %w", err)
	}
	return string(logBody), nil
}

// getWorkspaceID resolves a workspace name to its TFC ID.
func (c *TFCClient) getWorkspaceID(ctx context.Context, name string) (string, error) {
	url := fmt.Sprintf("https://%s/api/v2/organizations/%s/workspaces/%s", c.Hostname, c.Org, name)
	body, err := c.apiGet(ctx, url)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.Data.ID, nil
}

// apiGet performs an authenticated GET request to the TFC API.
func (c *TFCClient) apiGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

func TestGetInfo(t *testing.T) {
	p := &SentryPlugin{}
	info := p.GetInfo()

	if info.Name != "sentry" {
		t.Errorf("expected name 'sentry', got %q", info.Name)
	}

	if info.Version == "" {
		t.Error("expected non-empty version")
	}

	if len(info.Hooks) == 0 {
		t.Error("expected at least one hook")
	}

	// Check hooks include expected values
	hasPrePublish := false
	hasPostPublish := false
	for _, hook := range info.Hooks {
		if hook == plugin.HookPrePublish {
			hasPrePublish = true
		}
		if hook == plugin.HookPostPublish {
			hasPostPublish = true
		}
	}
	if !hasPrePublish {
		t.Error("expected HookPrePublish in hooks")
	}
	if !hasPostPublish {
		t.Error("expected HookPostPublish in hooks")
	}
}

func TestParseConfig(t *testing.T) {
	p := &SentryPlugin{}

	tests := []struct {
		name   string
		config map[string]any
		check  func(*Config) bool
	}{
		{
			name: "with all fields",
			config: map[string]any{
				"auth_token":        "test-token",
				"org":               "my-org",
				"project":           "my-project",
				"url":               "https://custom.sentry.io",
				"version_format":    "v{{.Version}}",
				"environment":       "staging",
				"set_commits":       false,
				"create_deploy":     false,
				"upload_sourcemaps": true,
				"finalize":          false,
			},
			check: func(cfg *Config) bool {
				return cfg.AuthToken == "test-token" &&
					cfg.Org == "my-org" &&
					cfg.Project == "my-project" &&
					cfg.URL == "https://custom.sentry.io" &&
					cfg.VersionFormat == "v{{.Version}}" &&
					cfg.Environment == "staging" &&
					cfg.SetCommits == false &&
					cfg.CreateDeploy == false &&
					cfg.UploadSourcemaps == true &&
					cfg.Finalize == false
			},
		},
		{
			name:   "with defaults",
			config: map[string]any{},
			check: func(cfg *Config) bool {
				return cfg.URL == "https://sentry.io" &&
					cfg.VersionFormat == "{{.Version}}" &&
					cfg.Environment == "production" &&
					cfg.SetCommits == true &&
					cfg.CreateDeploy == true &&
					cfg.UploadSourcemaps == false &&
					cfg.Finalize == true
			},
		},
		{
			name: "with multiple projects",
			config: map[string]any{
				"projects": []any{"frontend", "backend", "api"},
			},
			check: func(cfg *Config) bool {
				return len(cfg.Projects) == 3 &&
					cfg.Projects[0] == "frontend" &&
					cfg.Projects[1] == "backend" &&
					cfg.Projects[2] == "api"
			},
		},
		{
			name: "with commits config",
			config: map[string]any{
				"commits": map[string]any{
					"auto":       false,
					"repository": "org/repo",
				},
			},
			check: func(cfg *Config) bool {
				return cfg.Commits.Auto == false &&
					cfg.Commits.Repository == "org/repo"
			},
		},
		{
			name: "with deploy config",
			config: map[string]any{
				"deploy": map[string]any{
					"environment": "staging",
					"name":        "Staging Deploy",
				},
			},
			check: func(cfg *Config) bool {
				return cfg.Deploy.Environment == "staging" &&
					cfg.Deploy.Name == "Staging Deploy"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := p.parseConfig(tt.config)
			if !tt.check(cfg) {
				t.Errorf("parseConfig() did not produce expected config")
			}
		})
	}
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected []string
	}{
		{
			name: "single project",
			config: &Config{
				Project: "my-project",
			},
			expected: []string{"my-project"},
		},
		{
			name: "multiple projects",
			config: &Config{
				Projects: []string{"frontend", "backend"},
			},
			expected: []string{"frontend", "backend"},
		},
		{
			name: "project and projects combined",
			config: &Config{
				Project:  "api",
				Projects: []string{"frontend", "backend"},
			},
			expected: []string{"frontend", "backend", "api"},
		},
		{
			name: "project already in projects",
			config: &Config{
				Project:  "frontend",
				Projects: []string{"frontend", "backend"},
			},
			expected: []string{"frontend", "backend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.getProjects()
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d projects, got %d", len(tt.expected), len(result))
				return
			}
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("expected project %d to be %s, got %s", i, expected, result[i])
				}
			}
		})
	}
}

func TestFormatVersion(t *testing.T) {
	p := &SentryPlugin{}

	releaseCtx := plugin.ReleaseContext{
		Version:   "1.2.3",
		TagName:   "v1.2.3",
		CommitSHA: "abc123def456789",
	}

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{
			name:     "version only",
			format:   "{{.Version}}",
			expected: "1.2.3",
		},
		{
			name:     "with prefix",
			format:   "v{{.Version}}",
			expected: "v1.2.3",
		},
		{
			name:     "tag name",
			format:   "{{.TagName}}",
			expected: "v1.2.3",
		},
		{
			name:     "short SHA",
			format:   "{{.Version}}-{{.ShortSHA}}",
			expected: "1.2.3-abc123d",
		},
		{
			name:     "complex format",
			format:   "release-{{.Version}}-{{.ShortSHA}}",
			expected: "release-1.2.3-abc123d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.formatVersion(tt.format, releaseCtx)
			if err != nil {
				t.Fatalf("formatVersion() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("formatVersion() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc123def456789", "abc123d"},
		{"abc", "abc"},
		{"", ""},
		{"1234567", "1234567"},
		{"12345678", "1234567"},
	}

	for _, tt := range tests {
		result := shortSHA(tt.input)
		if result != tt.expected {
			t.Errorf("shortSHA(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestValidate(t *testing.T) {
	p := &SentryPlugin{}
	ctx := context.Background()

	tests := []struct {
		name      string
		config    map[string]any
		wantValid bool
	}{
		{
			name: "missing auth token",
			config: map[string]any{
				"org":     "my-org",
				"project": "my-project",
			},
			wantValid: false,
		},
		{
			name: "missing org",
			config: map[string]any{
				"auth_token": "test-token",
				"project":    "my-project",
			},
			wantValid: false,
		},
		{
			name: "missing project",
			config: map[string]any{
				"auth_token": "test-token",
				"org":        "my-org",
			},
			wantValid: false,
		},
		{
			name: "invalid version format",
			config: map[string]any{
				"auth_token":     "test-token",
				"org":            "my-org",
				"project":        "my-project",
				"version_format": "{{.Invalid",
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := p.Validate(ctx, tt.config)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			isValid := len(resp.Errors) == 0
			if isValid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v; errors: %v", isValid, tt.wantValid, resp.Errors)
			}
		})
	}
}

func TestExecutePrePublishDryRun(t *testing.T) {
	p := &SentryPlugin{}
	ctx := context.Background()

	releaseCtx := plugin.ReleaseContext{
		Version: "1.0.0",
		TagName: "v1.0.0",
		Branch:  "main",
	}

	req := plugin.ExecuteRequest{
		Hook:   plugin.HookPrePublish,
		DryRun: true,
		Config: map[string]any{
			"auth_token": "test-token",
			"org":        "my-org",
			"project":    "my-project",
		},
		Context: releaseCtx,
	}

	resp, err := p.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !resp.Success {
		t.Errorf("Execute() success = false, want true")
	}

	if !strings.Contains(resp.Message, "Would create Sentry release") {
		t.Errorf("Execute() message should mention creating release, got: %s", resp.Message)
	}

	if !strings.Contains(resp.Message, "1.0.0") {
		t.Errorf("Execute() message should contain version, got: %s", resp.Message)
	}
}

func TestExecutePostPublishDryRun(t *testing.T) {
	p := &SentryPlugin{}
	ctx := context.Background()

	releaseCtx := plugin.ReleaseContext{
		Version: "1.0.0",
		TagName: "v1.0.0",
		Branch:  "main",
	}

	req := plugin.ExecuteRequest{
		Hook:   plugin.HookPostPublish,
		DryRun: true,
		Config: map[string]any{
			"auth_token":    "test-token",
			"org":           "my-org",
			"project":       "my-project",
			"set_commits":   true,
			"create_deploy": true,
			"finalize":      true,
		},
		Context: releaseCtx,
	}

	resp, err := p.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !resp.Success {
		t.Errorf("Execute() success = false, want true")
	}

	if !strings.Contains(resp.Message, "commits") {
		t.Errorf("Execute() message should mention commits, got: %s", resp.Message)
	}

	if !strings.Contains(resp.Message, "deploy") {
		t.Errorf("Execute() message should mention deploy, got: %s", resp.Message)
	}

	if !strings.Contains(resp.Message, "finalize") {
		t.Errorf("Execute() message should mention finalize, got: %s", resp.Message)
	}
}

func TestExtractCommits(t *testing.T) {
	p := &SentryPlugin{}

	cfg := &Config{
		Commits: CommitsConfig{
			Repository: "org/repo",
		},
	}

	releaseCtx := plugin.ReleaseContext{
		Changes: &plugin.CategorizedChanges{
			Features: []plugin.ConventionalCommit{
				{Hash: "abc123", Type: "feat", Description: "Add feature"},
			},
			Fixes: []plugin.ConventionalCommit{
				{Hash: "def456", Type: "fix", Description: "Fix bug"},
			},
		},
	}

	commits := p.extractCommits(cfg, releaseCtx)

	if len(commits) != 2 {
		t.Errorf("expected 2 commits, got %d", len(commits))
		return
	}

	if commits[0].ID != "abc123" {
		t.Errorf("expected first commit ID 'abc123', got '%s'", commits[0].ID)
	}
	if commits[0].Repository != "org/repo" {
		t.Errorf("expected repository 'org/repo', got '%s'", commits[0].Repository)
	}
}

func TestSentryClientGetOrganization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		response := map[string]any{
			"id":   "org-123",
			"slug": "my-org",
			"name": "My Organization",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &SentryClient{
		baseURL:    server.URL,
		authToken:  "test-token",
		org:        "my-org",
		httpClient: http.DefaultClient,
	}

	org, err := client.GetOrganization(context.Background())
	if err != nil {
		t.Fatalf("GetOrganization() error = %v", err)
	}

	if org.Slug != "my-org" {
		t.Errorf("Expected org slug 'my-org', got '%s'", org.Slug)
	}
}

func TestSentryClientCreateRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		response := map[string]any{
			"version":     "1.0.0",
			"dateCreated": "2024-01-01T00:00:00Z",
			"projects": []map[string]any{
				{"slug": "my-project"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &SentryClient{
		baseURL:    server.URL,
		authToken:  "test-token",
		org:        "my-org",
		httpClient: http.DefaultClient,
	}

	release, err := client.CreateRelease(context.Background(), "1.0.0", []string{"my-project"})
	if err != nil {
		t.Fatalf("CreateRelease() error = %v", err)
	}

	if release.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", release.Version)
	}
}

func TestSentryClientCreateDeploy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"id":          "deploy-123",
			"environment": "production",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &SentryClient{
		baseURL:    server.URL,
		authToken:  "test-token",
		org:        "my-org",
		httpClient: http.DefaultClient,
	}

	deploy, err := client.CreateDeploy(context.Background(), "1.0.0", DeployConfig{Environment: "production"})
	if err != nil {
		t.Fatalf("CreateDeploy() error = %v", err)
	}

	if deploy.Environment != "production" {
		t.Errorf("Expected environment 'production', got '%s'", deploy.Environment)
	}
}

func TestSentryClientFinalizeRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &SentryClient{
		baseURL:    server.URL,
		authToken:  "test-token",
		org:        "my-org",
		httpClient: http.DefaultClient,
	}

	err := client.FinalizeRelease(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("FinalizeRelease() error = %v", err)
	}
}

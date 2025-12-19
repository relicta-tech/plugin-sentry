// Package main implements the Sentry plugin for Relicta.
package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// Version is set at build time.
var Version = "0.1.0"

// SentryPlugin implements the plugin.Plugin interface for Sentry integration.
type SentryPlugin struct{}

// Config represents Sentry plugin configuration.
type Config struct {
	AuthToken        string           `json:"auth_token"`
	Org              string           `json:"org"`
	Project          string           `json:"project"`
	Projects         []string         `json:"projects"`
	URL              string           `json:"url"`
	VersionFormat    string           `json:"version_format"`
	Environment      string           `json:"environment"`
	SetCommits       bool             `json:"set_commits"`
	Commits          CommitsConfig    `json:"commits"`
	CreateDeploy     bool             `json:"create_deploy"`
	Deploy           DeployConfig     `json:"deploy"`
	UploadSourcemaps bool             `json:"upload_sourcemaps"`
	Sourcemaps       SourcemapsConfig `json:"sourcemaps"`
	Finalize         bool             `json:"finalize"`
}

// CommitsConfig contains commit association settings.
type CommitsConfig struct {
	Auto       bool   `json:"auto"`
	Repository string `json:"repository"`
}

// DeployConfig contains deploy tracking settings.
type DeployConfig struct {
	Environment string `json:"environment"`
	Name        string `json:"name,omitempty"`
}

// SourcemapsConfig contains source map upload settings.
type SourcemapsConfig struct {
	Path      string   `json:"path"`
	URLPrefix string   `json:"url_prefix"`
	Include   []string `json:"include"`
	Exclude   []string `json:"exclude"`
}

// GetInfo returns plugin metadata.
func (p *SentryPlugin) GetInfo() plugin.Info {
	return plugin.Info{
		Name:        "sentry",
		Version:     Version,
		Description: "Sentry release tracking, deploy notifications, and commit association",
		Author:      "Relicta",
		Hooks: []plugin.Hook{
			plugin.HookPrePublish,
			plugin.HookPostPublish,
			plugin.HookOnError,
		},
	}
}

// Execute handles plugin execution for the specified hook.
func (p *SentryPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	cfg := p.parseConfig(req.Config)

	switch req.Hook {
	case plugin.HookPrePublish:
		return p.handlePrePublish(ctx, cfg, req.Context, req.DryRun)
	case plugin.HookPostPublish:
		return p.handlePostPublish(ctx, cfg, req.Context, req.DryRun)
	case plugin.HookOnError:
		return p.handleOnError(ctx, cfg, req.Context, req.DryRun)
	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not implemented", req.Hook),
		}, nil
	}
}

// Validate validates the plugin configuration.
func (p *SentryPlugin) Validate(ctx context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()
	cfg := p.parseConfig(config)

	// Validate auth token
	if cfg.AuthToken == "" {
		vb.AddError("auth_token", "Sentry auth token is required")
		return vb.Build(), nil
	}

	// Validate organization
	if cfg.Org == "" {
		vb.AddError("org", "Sentry organization is required")
	}

	// Validate projects
	projects := cfg.getProjects()
	if len(projects) == 0 {
		vb.AddError("project", "At least one project is required")
	}

	// Validate version format template
	if cfg.VersionFormat != "" {
		_, err := template.New("").Parse(cfg.VersionFormat)
		if err != nil {
			vb.AddError("version_format", fmt.Sprintf("Invalid version format template: %v", err))
		}
	}

	// Test API connectivity if auth token is provided
	if cfg.AuthToken != "" && cfg.Org != "" {
		client := NewSentryClient(cfg.URL, cfg.AuthToken, cfg.Org)
		if _, err := client.GetOrganization(ctx); err != nil {
			vb.AddError("auth_token", fmt.Sprintf("Failed to authenticate with Sentry: %v", err))
		}
	}

	return vb.Build(), nil
}

// parseConfig parses and applies defaults to the configuration.
func (p *SentryPlugin) parseConfig(raw map[string]any) *Config {
	parser := helpers.NewConfigParser(raw)

	cfg := &Config{
		AuthToken:        parser.GetString("auth_token", "SENTRY_AUTH_TOKEN", ""),
		Org:              parser.GetString("org", "SENTRY_ORG", ""),
		Project:          parser.GetString("project", "SENTRY_PROJECT", ""),
		URL:              parser.GetString("url", "SENTRY_URL", "https://sentry.io"),
		VersionFormat:    parser.GetString("version_format", "", "{{.Version}}"),
		Environment:      parser.GetString("environment", "", "production"),
		SetCommits:       parser.GetBool("set_commits", true),
		CreateDeploy:     parser.GetBool("create_deploy", true),
		UploadSourcemaps: parser.GetBool("upload_sourcemaps", false),
		Finalize:         parser.GetBool("finalize", true),
	}

	// Parse projects array
	if projects, ok := raw["projects"].([]any); ok {
		for _, p := range projects {
			if s, ok := p.(string); ok {
				cfg.Projects = append(cfg.Projects, s)
			}
		}
	}

	// Parse commits config
	if commits, ok := raw["commits"].(map[string]any); ok {
		commitParser := helpers.NewConfigParser(commits)
		cfg.Commits = CommitsConfig{
			Auto:       commitParser.GetBool("auto", true),
			Repository: commitParser.GetString("repository", "", ""),
		}
	} else {
		cfg.Commits = CommitsConfig{Auto: true}
	}

	// Parse deploy config
	if deploy, ok := raw["deploy"].(map[string]any); ok {
		deployParser := helpers.NewConfigParser(deploy)
		cfg.Deploy = DeployConfig{
			Environment: deployParser.GetString("environment", "", cfg.Environment),
			Name:        deployParser.GetString("name", "", ""),
		}
	} else {
		cfg.Deploy = DeployConfig{
			Environment: cfg.Environment,
		}
	}

	// Parse sourcemaps config
	if sourcemaps, ok := raw["sourcemaps"].(map[string]any); ok {
		smParser := helpers.NewConfigParser(sourcemaps)
		cfg.Sourcemaps = SourcemapsConfig{
			Path:      smParser.GetString("path", "", "./dist"),
			URLPrefix: smParser.GetString("url_prefix", "", "~/"),
		}
		if include, ok := sourcemaps["include"].([]any); ok {
			for _, i := range include {
				if s, ok := i.(string); ok {
					cfg.Sourcemaps.Include = append(cfg.Sourcemaps.Include, s)
				}
			}
		}
		if exclude, ok := sourcemaps["exclude"].([]any); ok {
			for _, e := range exclude {
				if s, ok := e.(string); ok {
					cfg.Sourcemaps.Exclude = append(cfg.Sourcemaps.Exclude, s)
				}
			}
		}
	}

	return cfg
}

// getProjects returns all configured projects.
func (cfg *Config) getProjects() []string {
	projects := cfg.Projects
	if cfg.Project != "" {
		// Check if already in list
		found := false
		for _, p := range projects {
			if p == cfg.Project {
				found = true
				break
			}
		}
		if !found {
			projects = append(projects, cfg.Project)
		}
	}
	return projects
}

// formatVersion renders the version string using the template.
func (p *SentryPlugin) formatVersion(format string, ctx plugin.ReleaseContext) (string, error) {
	tmpl, err := template.New("version").Parse(format)
	if err != nil {
		return "", err
	}

	data := struct {
		Version  string
		TagName  string
		ShortSHA string
	}{
		Version:  ctx.Version,
		TagName:  ctx.TagName,
		ShortSHA: shortSHA(ctx.CommitSHA),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// shortSHA returns the first 7 characters of a SHA.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// handlePrePublish creates the release in Sentry before publishing.
func (p *SentryPlugin) handlePrePublish(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	version, err := p.formatVersion(cfg.VersionFormat, releaseCtx)
	if err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to format version: %v", err),
		}, nil
	}

	projects := cfg.getProjects()

	if dryRun {
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Would create Sentry release '%s' for projects: %s", version, strings.Join(projects, ", ")),
			Outputs: map[string]any{
				"version":  version,
				"projects": projects,
			},
		}, nil
	}

	client := NewSentryClient(cfg.URL, cfg.AuthToken, cfg.Org)

	// Create release
	release, err := client.CreateRelease(ctx, version, projects)
	if err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create release: %v", err),
		}, nil
	}

	return &plugin.ExecuteResponse{
		Success: true,
		Message: fmt.Sprintf("Created Sentry release: %s", release.Version),
		Outputs: map[string]any{
			"version":      release.Version,
			"release_url":  release.URL,
			"date_created": release.DateCreated,
		},
	}, nil
}

// handlePostPublish finalizes the release and creates deploy record.
func (p *SentryPlugin) handlePostPublish(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	version, err := p.formatVersion(cfg.VersionFormat, releaseCtx)
	if err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to format version: %v", err),
		}, nil
	}

	var results []string

	if dryRun {
		if cfg.SetCommits {
			results = append(results, "Would associate commits with release")
		}
		if cfg.CreateDeploy {
			results = append(results, fmt.Sprintf("Would create deploy for environment: %s", cfg.Deploy.Environment))
		}
		if cfg.Finalize {
			results = append(results, "Would finalize release")
		}

		return &plugin.ExecuteResponse{
			Success: true,
			Message: strings.Join(results, "; "),
			Outputs: map[string]any{
				"version": version,
			},
		}, nil
	}

	client := NewSentryClient(cfg.URL, cfg.AuthToken, cfg.Org)

	// Associate commits
	if cfg.SetCommits {
		commits := p.extractCommits(cfg, releaseCtx)
		if len(commits) > 0 {
			if err := client.SetCommits(ctx, version, commits); err != nil {
				results = append(results, fmt.Sprintf("Warning: Failed to set commits: %v", err))
			} else {
				results = append(results, fmt.Sprintf("Associated %d commits", len(commits)))
			}
		}
	}

	// Create deploy
	if cfg.CreateDeploy {
		deploy, err := client.CreateDeploy(ctx, version, cfg.Deploy)
		if err != nil {
			results = append(results, fmt.Sprintf("Warning: Failed to create deploy: %v", err))
		} else {
			results = append(results, fmt.Sprintf("Created deploy: %s", deploy.Environment))
		}
	}

	// Finalize release
	if cfg.Finalize {
		if err := client.FinalizeRelease(ctx, version); err != nil {
			results = append(results, fmt.Sprintf("Warning: Failed to finalize release: %v", err))
		} else {
			results = append(results, "Finalized release")
		}
	}

	if len(results) == 0 {
		results = append(results, "No actions taken")
	}

	return &plugin.ExecuteResponse{
		Success: true,
		Message: strings.Join(results, "; "),
		Outputs: map[string]any{
			"version": version,
		},
	}, nil
}

// handleOnError handles release failure.
func (p *SentryPlugin) handleOnError(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	// For now, just log that an error occurred
	// Could be extended to update release status or create an issue
	return &plugin.ExecuteResponse{
		Success: true,
		Message: "Release failure noted (no Sentry action taken)",
	}, nil
}

// extractCommits extracts commit information from the release context.
func (p *SentryPlugin) extractCommits(cfg *Config, releaseCtx plugin.ReleaseContext) []CommitSpec {
	var commits []CommitSpec

	if releaseCtx.Changes == nil {
		return commits
	}

	repository := cfg.Commits.Repository
	if repository == "" {
		// Try to detect from git remote
		repository = "unknown"
	}

	// Collect all commits
	allCommits := append([]plugin.ConventionalCommit{}, releaseCtx.Changes.Features...)
	allCommits = append(allCommits, releaseCtx.Changes.Fixes...)
	allCommits = append(allCommits, releaseCtx.Changes.Breaking...)
	allCommits = append(allCommits, releaseCtx.Changes.Other...)

	for _, c := range allCommits {
		commits = append(commits, CommitSpec{
			ID:         c.Hash,
			Repository: repository,
			Message:    c.Description,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		})
	}

	return commits
}

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultTimeout = 30 * time.Second

// SentryClient wraps the Sentry API.
type SentryClient struct {
	baseURL    string
	authToken  string
	org        string
	httpClient *http.Client
}

// NewSentryClient creates a new Sentry API client.
func NewSentryClient(baseURL, authToken, org string) *SentryClient {
	if baseURL == "" {
		baseURL = "https://sentry.io"
	}
	return &SentryClient{
		baseURL:   baseURL,
		authToken: authToken,
		org:       org,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

// Release represents a Sentry release.
type Release struct {
	Version      string    `json:"version"`
	ShortVersion string    `json:"shortVersion,omitempty"`
	Ref          string    `json:"ref,omitempty"`
	URL          string    `json:"url,omitempty"`
	DateCreated  time.Time `json:"dateCreated,omitempty"`
	DateReleased time.Time `json:"dateReleased,omitempty"`
	Projects     []Project `json:"projects,omitempty"`
}

// Project represents a Sentry project.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Deploy represents a Sentry deploy.
type Deploy struct {
	ID           string    `json:"id"`
	Environment  string    `json:"environment"`
	Name         string    `json:"name,omitempty"`
	DateStarted  time.Time `json:"dateStarted,omitempty"`
	DateFinished time.Time `json:"dateFinished,omitempty"`
}

// Organization represents a Sentry organization.
type Organization struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// CommitSpec represents a commit for association.
type CommitSpec struct {
	ID          string `json:"id"`
	Repository  string `json:"repository"`
	Message     string `json:"message,omitempty"`
	AuthorName  string `json:"author_name,omitempty"`
	AuthorEmail string `json:"author_email,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

// CreateReleaseRequest represents the request to create a release.
type CreateReleaseRequest struct {
	Version     string   `json:"version"`
	Ref         string   `json:"ref,omitempty"`
	URL         string   `json:"url,omitempty"`
	Projects    []string `json:"projects"`
	DateStarted string   `json:"dateStarted,omitempty"`
}

// SetCommitsRequest represents the request to set commits.
type SetCommitsRequest struct {
	Commits []CommitSpec `json:"commits"`
}

// APIError represents a Sentry API error.
type APIError struct {
	Detail string `json:"detail"`
}

// request makes an HTTP request to the Sentry API.
func (c *SentryClient) request(ctx context.Context, method, endpoint string, body any, result any) error {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	fullURL := c.baseURL + "/api/0" + endpoint
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Detail != "" {
			return fmt.Errorf("API error: %s (status %d)", apiErr.Detail, resp.StatusCode)
		}
		return fmt.Errorf("API error: %s (status %d)", string(respBody), resp.StatusCode)
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// GetOrganization gets the configured organization.
func (c *SentryClient) GetOrganization(ctx context.Context) (*Organization, error) {
	endpoint := fmt.Sprintf("/organizations/%s/", c.org)
	var org Organization
	if err := c.request(ctx, http.MethodGet, endpoint, nil, &org); err != nil {
		return nil, err
	}
	return &org, nil
}

// CreateRelease creates a new release in Sentry.
func (c *SentryClient) CreateRelease(ctx context.Context, version string, projects []string) (*Release, error) {
	endpoint := fmt.Sprintf("/organizations/%s/releases/", c.org)

	req := CreateReleaseRequest{
		Version:     version,
		Projects:    projects,
		DateStarted: time.Now().UTC().Format(time.RFC3339),
	}

	var release Release
	if err := c.request(ctx, http.MethodPost, endpoint, req, &release); err != nil {
		// Check if release already exists
		if existingRelease, getErr := c.GetRelease(ctx, version); getErr == nil {
			return existingRelease, nil
		}
		return nil, err
	}
	return &release, nil
}

// GetRelease gets an existing release.
func (c *SentryClient) GetRelease(ctx context.Context, version string) (*Release, error) {
	endpoint := fmt.Sprintf("/organizations/%s/releases/%s/", c.org, url.PathEscape(version))
	var release Release
	if err := c.request(ctx, http.MethodGet, endpoint, nil, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

// SetCommits associates commits with a release.
func (c *SentryClient) SetCommits(ctx context.Context, version string, commits []CommitSpec) error {
	endpoint := fmt.Sprintf("/organizations/%s/releases/%s/commits/", c.org, url.PathEscape(version))
	req := SetCommitsRequest{Commits: commits}
	return c.request(ctx, http.MethodPost, endpoint, req, nil)
}

// CreateDeploy creates a deploy record for a release.
func (c *SentryClient) CreateDeploy(ctx context.Context, version string, deploy DeployConfig) (*Deploy, error) {
	endpoint := fmt.Sprintf("/organizations/%s/releases/%s/deploys/", c.org, url.PathEscape(version))

	req := map[string]any{
		"environment":  deploy.Environment,
		"dateStarted":  time.Now().UTC().Format(time.RFC3339),
		"dateFinished": time.Now().UTC().Format(time.RFC3339),
	}
	if deploy.Name != "" {
		req["name"] = deploy.Name
	}

	var result Deploy
	if err := c.request(ctx, http.MethodPost, endpoint, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FinalizeRelease marks a release as finalized.
func (c *SentryClient) FinalizeRelease(ctx context.Context, version string) error {
	endpoint := fmt.Sprintf("/organizations/%s/releases/%s/", c.org, url.PathEscape(version))
	req := map[string]any{
		"dateReleased": time.Now().UTC().Format(time.RFC3339),
	}
	return c.request(ctx, http.MethodPut, endpoint, req, nil)
}

// GetProject gets project details.
func (c *SentryClient) GetProject(ctx context.Context, projectSlug string) (*Project, error) {
	endpoint := fmt.Sprintf("/projects/%s/%s/", c.org, projectSlug)
	var project Project
	if err := c.request(ctx, http.MethodGet, endpoint, nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

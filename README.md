# Relicta Sentry Plugin

A Relicta plugin that integrates with Sentry for release tracking, deploy notifications, and commit association.

## Features

- **Release Tracking**: Create and finalize Sentry releases with version info
- **Commit Association**: Link commits to releases for suspect commit tracking
- **Deploy Tracking**: Track deployments to different environments
- **Multiple Projects**: Support releases across multiple Sentry projects
- **Self-Hosted**: Support for self-hosted Sentry instances

## Installation

Download the appropriate binary for your platform from the [releases page](https://github.com/relicta-tech/plugin-sentry/releases).

## Configuration

Add the Sentry plugin to your `relicta.yaml`:

```yaml
plugins:
  - name: sentry
    enabled: true
    hooks:
      - PrePublish
      - PostPublish
    config:
      # Auth token (required, use environment variable)
      auth_token: ${SENTRY_AUTH_TOKEN}

      # Organization slug (required)
      org: "my-organization"

      # Project slug (at least one required)
      project: "my-project"
      # Or multiple projects
      projects:
        - "frontend"
        - "backend"

      # Self-hosted Sentry URL (optional)
      url: "https://sentry.io"

      # Version format template
      version_format: "{{.Version}}"

      # Environment for deploy tracking
      environment: "production"

      # Associate commits with release
      set_commits: true

      # Commit options
      commits:
        auto: true
        repository: "org/repo"

      # Create deploy record
      create_deploy: true

      # Deploy settings
      deploy:
        environment: "production"
        name: "Production Deploy"

      # Finalize release after publish
      finalize: true
```

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `SENTRY_AUTH_TOKEN` | Sentry auth token | Yes |
| `SENTRY_ORG` | Default organization | No |
| `SENTRY_PROJECT` | Default project | No |
| `SENTRY_URL` | Self-hosted URL | No |

## Getting an Auth Token

1. Go to Sentry Settings > Account > API Tokens (or Organization Settings > Auth Tokens)
2. Click "Create New Token"
3. Select the required scopes:
   - `project:releases`
   - `org:read`
4. Copy the token and store it securely as an environment variable

## Version Format

The `version_format` supports Go templates with the following variables:

| Variable | Description |
|----------|-------------|
| `{{.Version}}` | Release version (e.g., "1.2.3") |
| `{{.TagName}}` | Git tag name (e.g., "v1.2.3") |
| `{{.ShortSHA}}` | First 7 characters of commit SHA |

Examples:
- `{{.Version}}` -> "1.2.3"
- `v{{.Version}}` -> "v1.2.3"
- `{{.Version}}-{{.ShortSHA}}` -> "1.2.3-abc123d"

## Hooks

| Hook | Trigger | Action |
|------|---------|--------|
| `PrePublish` | Before release | Create release in Sentry |
| `PostPublish` | After successful release | Associate commits, create deploy, finalize |
| `OnError` | On release failure | Log failure |

## Commit Association

When `set_commits` is enabled, the plugin extracts commits from the release context and associates them with the Sentry release. This enables:

- Suspect commit detection
- Commit-level error tracking
- Release history with commit details

## Deploy Tracking

When `create_deploy` is enabled, the plugin creates a deploy record in Sentry that shows:

- Deployment environment
- Deploy timestamp
- Deploy duration
- Associated release

## Development

### Prerequisites

- Go 1.24+
- Sentry auth token for testing

### Building

```bash
go build -o sentry .
```

### Testing

```bash
go test -v ./...
```

### Linting

```bash
golangci-lint run
```

## License

MIT License - see [LICENSE](LICENSE) for details.

# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

Slack MCP Server is a Model Context Protocol (MCP) server for Slack Workspaces written in Go. It supports multiple transports (Stdio, SSE, HTTP) and authentication methods (User OAuth `xoxp`, Bot `xoxb`, or browser session `xoxc`/`xoxd` tokens).

## Build & Development Commands

```bash
# Build the project (cleans, tidies, formats, then builds)
make build

# Run unit tests
make test

# Run integration tests (requires Slack credentials)
make test-integration

# Run a single test
go test -count=1 -v -run="TestNameHere" ./pkg/handler/...

# Format code
make format

# Tidy dependencies
make tidy

# Download dependencies
make deps

# Clean build artifacts
make clean

# Build for all platforms (darwin/linux/windows, amd64/arm64)
make build-all-platforms

# Build DXT extension for Claude Desktop
make build-dxt

# Run with MCP Inspector for debugging
npx @modelcontextprotocol/inspector go run ./cmd/slack-mcp-server --transport stdio
```

## Architecture

### Entry Point & Server Initialization
- `cmd/slack-mcp-server/main.go` - CLI entry point, parses transport flag (`-t stdio|sse|http`), initializes provider and MCP server, starts user/channel cache watchers

### Core Packages
- `pkg/server/server.go` - MCP server setup, registers tools and resources using `mark3labs/mcp-go` library
- `pkg/provider/api.go` - `ApiProvider` manages Slack API clients, caching (users/channels), rate limiting
- `pkg/provider/edge/` - Edge API client for Enterprise Slack workspaces (handles non-standard API endpoints)

### MCP Tool Handlers
- `pkg/handler/conversations.go` - Handlers for `conversations_history`, `conversations_replies`, `conversations_add_message`, `conversations_search_messages`
- `pkg/handler/channels.go` - Handler for `channels_list` tool and channels resource

### Supporting Packages
- `pkg/transport/transport.go` - HTTP client configuration (proxy, TLS, custom user-agent)
- `pkg/limiter/limits.go` - Slack API rate limiter (Tier 2)
- `pkg/text/text_processor.go` - Text processing, message formatting, unfurling logic
- `pkg/server/auth/sse_auth.go` - Bearer token authentication for SSE/HTTP transports

### Slack Client Architecture
`MCPSlackClient` wraps both standard `slack-go/slack` client and an `edge.Client` for Enterprise features. Token type (`xoxp`/`xoxb`/`xoxc`) determines which API endpoints are used:
- Standard API: Used for OAuth tokens in non-enterprise setups
- Edge API: Used for `xoxc`/`xoxd` tokens in Enterprise Grid setups

### Caching Strategy
Users and channels are cached on startup to enable `@username` and `#channel` lookups. Cache files default to OS-specific locations:
- macOS: `~/Library/Caches/slack-mcp-server/`
- Linux: `~/.cache/slack-mcp-server/`

## Testing Conventions

- Unit tests: Name must contain `Unit` (e.g., `TestParseParamsUnit`)
- Integration tests: Name must contain `Integration` (e.g., `TestSearchIntegration`)
- Test files: `*_test.go` alongside source files

## Key Dependencies

- `github.com/mark3labs/mcp-go` - MCP protocol implementation
- `github.com/slack-go/slack` - Slack API client
- `github.com/rusq/slackdump/v3/auth` - Authentication provider for browser tokens
- `go.uber.org/zap` - Structured logging

## Environment Variables for Development

```bash
# Required (one of these auth methods)
SLACK_MCP_XOXP_TOKEN=xoxp-...   # User OAuth token
SLACK_MCP_XOXB_TOKEN=xoxb-...   # Bot token
SLACK_MCP_XOXC_TOKEN=xoxc-...   # Browser token (with XOXD)
SLACK_MCP_XOXD_TOKEN=xoxd-...   # Browser cookie

# Optional
SLACK_MCP_LOG_LEVEL=debug       # Log level (debug|info|warn|error)
SLACK_MCP_ADD_MESSAGE_TOOL=true # Enable posting messages
```

## Adding New MCP Tools

1. Add tool definition in `pkg/server/server.go` using `mcp.NewTool()` with parameters
2. Create handler method in appropriate handler file (`pkg/handler/`)
3. Register handler with `s.AddTool(tool, handler)`

## Bot Token Limitations

When using `xoxb-*` bot tokens:
- `conversations_search_messages` tool is not registered (bot tokens cannot use `search.messages` API)
- Bot must be explicitly invited to channels to access them

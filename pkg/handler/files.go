package handler

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// FilesHandler handles file-related MCP tools
type FilesHandler struct {
	provider *provider.ApiProvider
	logger   *zap.Logger
}

// NewFilesHandler creates a new files handler
func NewFilesHandler(provider *provider.ApiProvider, logger *zap.Logger) *FilesHandler {
	return &FilesHandler{
		provider: provider,
		logger:   logger,
	}
}

// FilesGetContentHandler retrieves and returns the content of a file
func (h *FilesHandler) FilesGetContentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID := request.GetString("file_id", "")
	if fileID == "" {
		return mcp.NewToolResultError("file_id is required"), nil
	}

	// Get file info first to get the download URL
	fileInfo, _, _, err := h.provider.Slack().Raw().Slack.GetFileInfoContext(ctx, fileID, 0, 0)
	if err != nil {
		h.logger.Error("Failed to get file info", zap.Error(err), zap.String("file_id", fileID))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get file info: %v", err)), nil
	}

	// Check file size - limit to 1MB for safety
	maxSize := 1024 * 1024 // 1MB
	if fileInfo.Size > maxSize {
		return mcp.NewToolResultError(fmt.Sprintf("File too large: %d bytes (max %d bytes). Use the file URL to download manually: %s", fileInfo.Size, maxSize, fileInfo.URLPrivate)), nil
	}

	// Determine if we should extract text or return raw content
	extractText := request.GetBool("extract_text", true)

	// Download the file content
	var buf bytes.Buffer
	err = h.provider.Slack().Raw().Slack.GetFileContext(ctx, fileInfo.URLPrivate, &buf)
	if err != nil {
		h.logger.Error("Failed to download file", zap.Error(err), zap.String("file_id", fileID))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to download file: %v", err)), nil
	}

	content := buf.String()

	// For HTML files, optionally extract just the text
	if extractText && isHTMLFile(fileInfo) {
		extracted := extractHTMLText(content)
		if extracted != "" {
			content = extracted
		}
	}

	// Build response with metadata
	response := fmt.Sprintf("=== File: %s ===\n", fileInfo.Name)
	response += fmt.Sprintf("Type: %s\n", fileInfo.Mimetype)
	response += fmt.Sprintf("Size: %d bytes\n", fileInfo.Size)
	if fileInfo.Title != "" && fileInfo.Title != fileInfo.Name {
		response += fmt.Sprintf("Title: %s\n", fileInfo.Title)
	}
	response += "=== Content ===\n"
	response += content

	return mcp.NewToolResultText(response), nil
}

// FilesListHandler lists files in a channel or from a user
func (h *FilesHandler) FilesListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := slack.GetFilesParameters{
		Count: 20,
	}

	channelID := request.GetString("channel_id", "")
	if channelID != "" {
		// Resolve channel name to ID if needed
		resolved := h.resolveChannelID(channelID)
		params.Channel = resolved
	}

	userID := request.GetString("user_id", "")
	if userID != "" {
		params.User = userID
	}

	types := request.GetString("types", "")
	if types != "" {
		params.Types = types
	}

	count := request.GetInt("count", 20)
	if count > 0 {
		params.Count = count
		if params.Count > 100 {
			params.Count = 100
		}
	}

	files, paging, err := h.provider.Slack().Raw().Slack.GetFilesContext(ctx, params)
	if err != nil {
		h.logger.Error("Failed to list files", zap.Error(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list files: %v", err)), nil
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d files (page %d of %d)\n\n", paging.Total, paging.Page, paging.Pages))

	for _, f := range files {
		sb.WriteString(fmt.Sprintf("📎 ID: %s\n", f.ID))
		sb.WriteString(fmt.Sprintf("   Name: %s\n", f.Name))
		if f.Title != "" && f.Title != f.Name {
			sb.WriteString(fmt.Sprintf("   Title: %s\n", f.Title))
		}
		sb.WriteString(fmt.Sprintf("   Type: %s\n", f.Mimetype))
		sb.WriteString(fmt.Sprintf("   Size: %s\n", formatFileSize(f.Size)))
		sb.WriteString(fmt.Sprintf("   Created: %s\n", f.Created.Time().Format("2006-01-02 15:04:05")))
		if f.Permalink != "" {
			sb.WriteString(fmt.Sprintf("   Permalink: %s\n", f.Permalink))
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// resolveChannelID resolves channel name (like #emails) to channel ID
func (h *FilesHandler) resolveChannelID(channel string) string {
	if strings.HasPrefix(channel, "#") || strings.HasPrefix(channel, "@") {
		channelsMaps := h.provider.ProvideChannelsMaps()
		if chn, ok := channelsMaps.ChannelsInv[channel]; ok {
			return channelsMaps.Channels[chn].ID
		}
		// If not found, return without the prefix as a fallback
		return strings.TrimPrefix(strings.TrimPrefix(channel, "#"), "@")
	}
	return channel
}

// isHTMLFile checks if the file is HTML
func isHTMLFile(f *slack.File) bool {
	return f.Mimetype == "text/html" ||
		strings.HasSuffix(strings.ToLower(f.Name), ".html") ||
		strings.HasSuffix(strings.ToLower(f.Name), ".htm")
}

// extractHTMLText extracts readable text from HTML content
func extractHTMLText(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	var sb strings.Builder
	var extractText func(*html.Node)

	extractText = func(n *html.Node) {
		// Skip script, style, and head tags
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "head", "noscript":
				return
			case "br", "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6":
				sb.WriteString("\n")
			case "td", "th":
				sb.WriteString("\t")
			}
		}

		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c)
		}

		// Add newline after block elements
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "table", "ul", "ol", "blockquote":
				sb.WriteString("\n")
			}
		}
	}

	extractText(doc)

	// Clean up multiple newlines and spaces
	result := sb.String()
	result = strings.ReplaceAll(result, "\t\n", "\t")
	result = strings.ReplaceAll(result, " \n", "\n")
	result = strings.ReplaceAll(result, "\n ", "\n")

	// Collapse multiple newlines
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(result)
}

// formatFileSize formats bytes into human-readable size
func formatFileSize(bytes int) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// Package handler provides MCP tool handlers for Slack operations
// This file adds attachment/block/file extraction support for rich content
// including forwarded emails.
package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

// ExtractMessageContent extracts all content from a Slack message including
// attachments (for forwarded emails), blocks, and files.
// Returns:
//   - fullText: Combined text from all sources (message text + attachments + blocks)
//   - attachmentText: Just the attachment content (useful for separate processing)
//   - filesInfo: JSON string with file metadata
//   - hasRichContent: True if message has attachments, blocks, or files
func ExtractMessageContent(msg slack.Message) (fullText string, attachmentText string, filesInfo string, hasRichContent bool) {
	var textParts []string

	// Start with basic message text
	if msg.Text != "" {
		textParts = append(textParts, msg.Text)
	}

	// Check for rich content
	hasRichContent = len(msg.Attachments) > 0 ||
		len(msg.Blocks.BlockSet) > 0 ||
		len(msg.Files) > 0

	// Extract attachment content (CRITICAL FOR FORWARDED EMAILS)
	if len(msg.Attachments) > 0 {
		attachmentText = ExtractAttachmentText(msg.Attachments)
		if attachmentText != "" {
			textParts = append(textParts, "\n--- EMAIL/ATTACHMENT CONTENT ---")
			textParts = append(textParts, attachmentText)
		}
	}

	// Extract block content (rich formatting)
	if len(msg.Blocks.BlockSet) > 0 {
		blockText := ExtractBlockText(msg.Blocks)
		if blockText != "" {
			// Avoid duplicating content already in text
			existingText := strings.Join(textParts, "")
			if !strings.Contains(existingText, blockText) {
				textParts = append(textParts, blockText)
			}
		}
	}

	// Extract file information
	if len(msg.Files) > 0 {
		filesInfo = ExtractFilesJSON(msg.Files)
		textParts = append(textParts, "\n--- ATTACHED FILES ---")
		textParts = append(textParts, FormatFilesReadable(msg.Files))
	}

	fullText = strings.Join(textParts, "\n")
	return
}

// ExtractAttachmentText pulls all text from Slack attachments.
// This is essential for forwarded email content which stores the email
// body, subject, and sender information in attachment fields.
func ExtractAttachmentText(attachments []slack.Attachment) string {
	var allParts []string

	for i, att := range attachments {
		var parts []string

		// Service name (e.g., "Email", "GitHub", etc.)
		if att.ServiceName != "" {
			parts = append(parts, "Service: "+att.ServiceName)
		}

		// Author (email sender for forwarded emails)
		if att.AuthorName != "" {
			parts = append(parts, "From: "+att.AuthorName)
		}
		if att.AuthorSubname != "" {
			parts = append(parts, "      "+att.AuthorSubname)
		}

		// Title (email subject for forwarded emails)
		if att.Title != "" {
			parts = append(parts, "Subject: "+att.Title)
		}

		// Title link
		if att.TitleLink != "" {
			parts = append(parts, "Link: "+att.TitleLink)
		}

		// Pretext (context before main content)
		if att.Pretext != "" {
			parts = append(parts, att.Pretext)
		}

		// Main text content (email body for forwarded emails)
		if att.Text != "" {
			parts = append(parts, "\n"+att.Text)
		}

		// Fallback text (used when other content isn't displayable)
		if att.Fallback != "" && att.Text == "" {
			parts = append(parts, att.Fallback)
		}

		// Fields (structured data - common in email headers)
		for _, field := range att.Fields {
			if field.Title != "" && field.Value != "" {
				parts = append(parts, field.Title+": "+field.Value)
			} else if field.Value != "" {
				parts = append(parts, field.Value)
			}
		}

		// Footer (often contains timestamp or source info)
		if att.Footer != "" {
			parts = append(parts, "\nFooter: "+att.Footer)
		}

		// Timestamp
		if att.Ts != "" {
			parts = append(parts, "Timestamp: "+string(att.Ts))
		}

		// Image URL
		if att.ImageURL != "" {
			parts = append(parts, "Image: "+att.ImageURL)
		}

		// Thumbnail URL
		if att.ThumbURL != "" {
			parts = append(parts, "Thumbnail: "+att.ThumbURL)
		}

		// Video URL (HTML embed)
		if att.VideoHTML != "" {
			parts = append(parts, "[Video Embedded]")
		}

		// Actions (interactive buttons)
		for _, action := range att.Actions {
			if action.Text != "" {
				parts = append(parts, "[Action: "+action.Text+"]")
			}
		}

		// Message blocks within attachment (modern format)
		if len(att.Blocks.BlockSet) > 0 {
			blockText := ExtractBlockText(att.Blocks)
			if blockText != "" {
				parts = append(parts, blockText)
			}
		}

		if len(parts) > 0 {
			if i > 0 {
				allParts = append(allParts, "\n---\n")
			}
			allParts = append(allParts, strings.Join(parts, "\n"))
		}
	}

	return strings.Join(allParts, "")
}

// ExtractBlockText handles Slack Block Kit format messages.
// Blocks are the modern way Slack formats rich content.
func ExtractBlockText(blocks slack.Blocks) string {
	var parts []string

	for _, block := range blocks.BlockSet {
		text := extractSingleBlock(block)
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n")
}

func extractSingleBlock(block slack.Block) string {
	switch b := block.(type) {
	case *slack.SectionBlock:
		return extractSectionBlock(b)
	case *slack.HeaderBlock:
		if b.Text != nil {
			return "## " + b.Text.Text
		}
	case *slack.ContextBlock:
		return extractContextBlock(b)
	case *slack.RichTextBlock:
		return extractRichTextBlock(b)
	case *slack.DividerBlock:
		return "---"
	case *slack.ImageBlock:
		if b.AltText != "" {
			return "[Image: " + b.AltText + "]"
		}
		return "[Image]"
	case *slack.FileBlock:
		return "[File Block]"
	case *slack.ActionBlock:
		return extractActionBlock(b)
	case *slack.InputBlock:
		if b.Label != nil {
			return "[Input: " + b.Label.Text + "]"
		}
		return "[Input Block]"
	}
	return ""
}

func extractSectionBlock(b *slack.SectionBlock) string {
	var parts []string
	if b.Text != nil {
		parts = append(parts, b.Text.Text)
	}
	for _, field := range b.Fields {
		if field != nil {
			parts = append(parts, field.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func extractContextBlock(b *slack.ContextBlock) string {
	var parts []string
	for _, elem := range b.ContextElements.Elements {
		switch e := elem.(type) {
		case *slack.TextBlockObject:
			parts = append(parts, e.Text)
		case *slack.ImageBlockElement:
			if e.AltText != "" {
				parts = append(parts, "["+e.AltText+"]")
			}
		}
	}
	return strings.Join(parts, " | ")
}

func extractActionBlock(b *slack.ActionBlock) string {
	var parts []string
	for _, elem := range b.Elements.ElementSet {
		switch e := elem.(type) {
		case *slack.ButtonBlockElement:
			if e.Text != nil {
				parts = append(parts, "[Button: "+e.Text.Text+"]")
			}
		case *slack.SelectBlockElement:
			if e.Placeholder != nil {
				parts = append(parts, "[Select: "+e.Placeholder.Text+"]")
			}
		}
	}
	return strings.Join(parts, " ")
}

func extractRichTextBlock(rtb *slack.RichTextBlock) string {
	var parts []string

	for _, elem := range rtb.Elements {
		switch e := elem.(type) {
		case *slack.RichTextSection:
			text := extractRichTextSection(e)
			if text != "" {
				parts = append(parts, text)
			}
		case *slack.RichTextList:
			text := extractRichTextList(e)
			if text != "" {
				parts = append(parts, text)
			}
		case *slack.RichTextQuote:
			text := extractRichTextQuote(e)
			if text != "" {
				parts = append(parts, text)
			}
		case *slack.RichTextPreformatted:
			text := extractRichTextPreformatted(e)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, "\n")
}

func extractRichTextSection(section *slack.RichTextSection) string {
	var parts []string
	for _, elem := range section.Elements {
		switch e := elem.(type) {
		case *slack.RichTextSectionTextElement:
			text := e.Text
			// Apply styling hints
			if e.Style != nil {
				if e.Style.Bold {
					text = "**" + text + "**"
				}
				if e.Style.Italic {
					text = "_" + text + "_"
				}
				if e.Style.Strike {
					text = "~~" + text + "~~"
				}
				if e.Style.Code {
					text = "`" + text + "`"
				}
			}
			parts = append(parts, text)
		case *slack.RichTextSectionLinkElement:
			if e.Text != "" {
				parts = append(parts, e.Text+" ("+e.URL+")")
			} else {
				parts = append(parts, e.URL)
			}
		case *slack.RichTextSectionUserElement:
			parts = append(parts, "@"+e.UserID)
		case *slack.RichTextSectionChannelElement:
			parts = append(parts, "#"+e.ChannelID)
		case *slack.RichTextSectionEmojiElement:
			parts = append(parts, ":"+e.Name+":")
		case *slack.RichTextSectionBroadcastElement:
			parts = append(parts, "@"+e.Range)
		case *slack.RichTextSectionDateElement:
			parts = append(parts, "[Date]")
		case *slack.RichTextSectionColorElement:
			parts = append(parts, "[Color: "+e.Value+"]")
		}
	}
	return strings.Join(parts, "")
}

func extractRichTextList(list *slack.RichTextList) string {
	var items []string
	for i, item := range list.Elements {
		prefix := "• "
		if list.Style == slack.RTEListOrdered {
			prefix = fmt.Sprintf("%d. ", i+1)
		}
		// Handle indentation
		if list.Indent > 0 {
			prefix = strings.Repeat("  ", list.Indent) + prefix
		}

		switch elem := item.(type) {
		case *slack.RichTextSection:
			text := extractRichTextSection(elem)
			items = append(items, prefix+text)
		}
	}
	return strings.Join(items, "\n")
}

func extractRichTextQuote(quote *slack.RichTextQuote) string {
	var parts []string
	for _, elem := range quote.Elements {
		switch e := elem.(type) {
		case *slack.RichTextSection:
			text := extractRichTextSection(e)
			// Add quote prefix to each line
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				parts = append(parts, "> "+line)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func extractRichTextPreformatted(pre *slack.RichTextPreformatted) string {
	var parts []string
	for _, elem := range pre.Elements {
		switch e := elem.(type) {
		case *slack.RichTextSection:
			text := extractRichTextSection(e)
			parts = append(parts, text)
		}
	}
	return "```\n" + strings.Join(parts, "\n") + "\n```"
}

// FileInfo represents extracted file metadata
type FileInfo struct {
	Name      string `json:"name"`
	Title     string `json:"title,omitempty"`
	Type      string `json:"mimetype"`
	Size      int    `json:"size"`
	URL       string `json:"url,omitempty"`
	Permalink string `json:"permalink,omitempty"`
}

// ExtractFilesJSON creates JSON representation of attached files
func ExtractFilesJSON(files []slack.File) string {
	infos := make([]FileInfo, 0, len(files))
	for _, f := range files {
		infos = append(infos, FileInfo{
			Name:      f.Name,
			Title:     f.Title,
			Type:      f.Mimetype,
			Size:      f.Size,
			URL:       f.URLPrivate,
			Permalink: f.Permalink,
		})
	}

	bytes, err := json.Marshal(infos)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

// FormatFilesReadable creates human-readable file list with icons
func FormatFilesReadable(files []slack.File) string {
	var parts []string
	for _, f := range files {
		// Choose icon based on type
		icon := "📎"
		switch {
		case strings.HasPrefix(f.Mimetype, "image/"):
			icon = "🖼️"
		case strings.HasPrefix(f.Mimetype, "video/"):
			icon = "🎬"
		case strings.HasPrefix(f.Mimetype, "audio/"):
			icon = "🎵"
		case f.Mimetype == "application/pdf":
			icon = "📄"
		case strings.Contains(f.Mimetype, "spreadsheet") || strings.Contains(f.Mimetype, "excel"):
			icon = "📊"
		case strings.Contains(f.Mimetype, "document") || strings.Contains(f.Mimetype, "word"):
			icon = "📝"
		case strings.Contains(f.Mimetype, "presentation") || strings.Contains(f.Mimetype, "powerpoint"):
			icon = "📽️"
		case strings.Contains(f.Mimetype, "zip") || strings.Contains(f.Mimetype, "compressed"):
			icon = "🗜️"
		}

		info := f.Name
		if f.Title != "" && f.Title != f.Name {
			info = f.Title + " (" + f.Name + ")"
		}
		if f.Size > 0 {
			info += " - " + formatSize(f.Size)
		}
		if f.Permalink != "" {
			info += "\n    " + f.Permalink
		}
		parts = append(parts, icon+" "+info)
	}
	return strings.Join(parts, "\n")
}

func formatSize(bytes int) string {
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

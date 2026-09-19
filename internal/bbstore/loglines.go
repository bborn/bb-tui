package bbstore

import (
	"fmt"
	"strings"

	"github.com/bborn/bb-tui/internal/bbapi"
)

func LogLineFor(row bbapi.TimelineRow) (string, string, bool) {
	switch row.Kind {
	case "conversation":
		text := strings.TrimSpace(row.Text)
		if text == "" {
			return "", "", false
		}
		if row.Role == "user" {
			return "user", text, true
		}
		return "text", text, true

	case "system":
		title := strings.TrimSpace(row.Title)
		if title == "" {
			return "", "", false
		}
		return "system", title, true

	case "work":
		return workLine(row)
	}
	return "", "", false
}

func workLine(row bbapi.TimelineRow) (string, string, bool) {
	lineType := "tool"
	if row.Status == "error" {
		lineType = "error"
	}

	switch row.WorkKind {
	case "command":
		command := collapse(row.Command)
		if command == "" {
			return "", "", false
		}
		suffix := ""
		if row.ExitCode != nil && *row.ExitCode != 0 {
			suffix = fmt.Sprintf("  (exit %d)", *row.ExitCode)
			lineType = "error"
		}
		return lineType, "$ " + command + suffix, true

	case "file-change":
		if row.Path == "" {
			return lineType, "edited a file", true
		}
		return lineType, "edit " + row.Path, true

	case "image-view", "image-generation":
		path := strings.TrimSpace(row.Path)
		if path == "" {
			return lineType, "an image", true
		}
		return "image", path, true

	case "file-read":
		if row.Path == "" {
			return lineType, "read a file", true
		}
		return lineType, "read " + row.Path, true

	case "search":
		return lineType, "search " + collapse(row.Query), true

	case "approval":
		return "question", approvalText(row), true

	case "question":
		text := collapse(row.Question)
		if text == "" {
			text = collapse(row.Title)
		}
		return "question", text, true
	}

	title := collapse(row.Title)
	if title == "" {
		title = row.WorkKind
	}
	if title == "" {
		return "", "", false
	}
	return lineType, title, true
}

func approvalText(row bbapi.TimelineRow) string {
	title := collapse(row.Title)
	if title == "" {
		title = "approval requested"
	}
	switch row.Status {
	case "granted":
		return title + " — approved"
	case "denied":
		return title + " — denied"
	case "interrupted":
		return title + " — interrupted"
	}
	return title + " — waiting for you"
}

func collapse(value string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(value), " "))
}

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Attachment is a file the composer will send alongside the message.
type Attachment struct {
	Path  string
	Name  string
	Image bool
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true, ".svg": true, ".heic": true,
}

// droppedPath matches what a terminal inserts when a file is dragged onto it:
// an absolute or home-relative path whose spaces are backslash-escaped, or the
// whole thing wrapped in quotes.
var droppedPath = regexp.MustCompile(`'[^']+'|"[^"]+"|(?:~|/)(?:\\.|[^\s'"])+`)

// ExtractAttachments pulls dropped file paths out of a message and returns the
// text with each one replaced by a short placeholder. A path is only taken when
// it names a file that exists, so prose containing a slash is left alone.
func ExtractAttachments(text string, startIndex int) (string, []Attachment) {
	var found []Attachment

	replaced := droppedPath.ReplaceAllStringFunc(text, func(match string) string {
		path := unescapePath(match)
		if path == "" {
			return match
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return match
		}

		attachment := Attachment{
			Path:  path,
			Name:  filepath.Base(path),
			Image: imageExtensions[strings.ToLower(filepath.Ext(path))],
		}
		found = append(found, attachment)

		label := "File"
		if attachment.Image {
			label = "Image"
		}
		return "[" + label + " #" + itoaPositive(startIndex+len(found)) + "]"
	})

	return replaced, found
}

func unescapePath(match string) string {
	value := strings.TrimSpace(match)
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') ||
			(value[0] == '"' && value[len(value)-1] == '"') {
			value = value[1 : len(value)-1]
		}
	}
	value = strings.ReplaceAll(value, `\ `, " ")
	value = strings.ReplaceAll(value, `\(`, "(")
	value = strings.ReplaceAll(value, `\)`, ")")
	value = strings.ReplaceAll(value, `\&`, "&")
	value = strings.ReplaceAll(value, `\'`, "'")

	if strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			value = filepath.Join(home, value[2:])
		}
	}
	if !strings.HasPrefix(value, "/") {
		return ""
	}
	return value
}

func itoaPositive(value int) string {
	if value <= 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

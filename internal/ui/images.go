package ui

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// TerminalImageSupport is which inline-image protocol this terminal speaks, if
// any. Terminals disagree completely here, so it is detected rather than
// assumed: iTerm2 has its own OSC 1337, Kitty and Ghostty share a different
// one, and everything else can only be given a link.
type TerminalImageSupport int

const (
	ImagesUnsupported TerminalImageSupport = iota
	ImagesITerm2
	ImagesKitty
)

var (
	imageSupportOnce sync.Once
	imageSupport     TerminalImageSupport
)

// DetectImageSupport reports what the current terminal can draw. tmux blocks
// the passthrough these protocols need unless it is configured for it, so a
// tmux session is treated as unsupported rather than drawing garbage.
func DetectImageSupport() TerminalImageSupport {
	imageSupportOnce.Do(func() {
		imageSupport = detectImageSupport(os.Getenv)
	})
	return imageSupport
}

func detectImageSupport(env func(string) string) TerminalImageSupport {
	if env("BB_TUI_NO_IMAGES") != "" {
		return ImagesUnsupported
	}
	// Inside tmux the escape has to be wrapped in a passthrough that is off by
	// default; without it the sequence is printed as text.
	if env("TMUX") != "" && env("BB_TUI_TMUX_PASSTHROUGH") == "" {
		return ImagesUnsupported
	}

	switch {
	case env("TERM_PROGRAM") == "iTerm.app":
		return ImagesITerm2
	case env("TERM_PROGRAM") == "WezTerm":
		return ImagesITerm2
	case env("KITTY_WINDOW_ID") != "":
		return ImagesKitty
	case strings.Contains(env("TERM"), "kitty"):
		return ImagesKitty
	case env("GHOSTTY_RESOURCES_DIR") != "":
		return ImagesKitty
	}
	return ImagesUnsupported
}

// RenderInlineImage returns the escape sequence that draws an image, or "" when
// the terminal cannot draw one.
func RenderInlineImage(path string, cellWidth int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	encoded := base64.StdEncoding.EncodeToString(data)

	switch DetectImageSupport() {
	case ImagesITerm2:
		return "\x1b]1337;File=inline=1;preserveAspectRatio=1;width=" +
			strconv.Itoa(cellWidth) + ";name=" +
			base64.StdEncoding.EncodeToString([]byte(filepath.Base(path))) +
			":" + encoded + "\a"
	case ImagesKitty:
		return "\x1b_Ga=T,f=100,c=" + strconv.Itoa(cellWidth) + ";" + encoded + "\x1b\\"
	}
	return ""
}

// ImageLine is what a thread shows for an image: the picture itself where the
// terminal can draw one, and a labelled, clickable chip everywhere else.
func ImageLine(path string, width int) string {
	name := filepath.Base(path)
	if inline := RenderInlineImage(path, minInt(maxInt(width-8, 10), 60)); inline != "" {
		return inline + "\n" + Dim.Render("  "+name)
	}
	label := Icon("🖼 ", "[image] ") + name
	return "  " + Hyperlink("file://"+path, label)
}

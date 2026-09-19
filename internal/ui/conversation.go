package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bborn/bb-tui/internal/db"
)

// ConversationView renders a thread the way the app does — as a conversation
// rather than TaskYou's timestamped execution log. A bb thread is a
// conversation; a log of tool calls buries the part a person came to read.
var ConversationView = false

func wrapText(value string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, paragraph := range strings.Split(value, "\n") {
		if strings.TrimSpace(paragraph) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(paragraph) {
			switch {
			case line == "":
				line = word
			case len(line)+1+len(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// renderConversation turns the thread's rows into something readable: what was
// said is given room and colour, what was done is collapsed to one dim line
// each, so a long run of tool calls never hides the reply after it.
// MessageAnchor is where a message starts in the rendered conversation, and
// which log row it came from, so a key can act on the message a reader is
// looking at rather than on a line number.
type MessageAnchor struct {
	Line     int
	LogIndex int
}

// renderConversationWithAnchors renders and records message boundaries. The
// boundaries are found by rendering each row on its own and counting lines,
// rather than by scanning the finished text for a glyph, so a reply that
// happens to begin with the same character cannot be mistaken for one.
func renderConversationWithAnchors(
	logs []*db.TaskLog,
	width int,
	focused bool,
	markdown func(string) (string, error),
) (string, []MessageAnchor) {
	var out strings.Builder
	var anchors []MessageAnchor
	line := 0

	for index, log := range logs {
		piece := renderConversation([]*db.TaskLog{log}, width, focused, markdown)
		if log.LineType == "user" || log.LineType == "text" {
			anchors = append(anchors, MessageAnchor{Line: line, LogIndex: index})
		}
		out.WriteString(piece)
		line += strings.Count(piece, "\n")
	}

	return out.String(), anchors
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func renderConversation(logs []*db.TaskLog, width int, focused bool, markdown func(string) (string, error)) string {
	if width < 20 {
		width = 20
	}
	bodyWidth := width - 4

	muted := ColorMuted
	if !focused {
		muted = lipgloss.Color("#4B5563")
	}

	userBar := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	userText := lipgloss.NewStyle().Foreground(ColorPrimary)
	agentText := lipgloss.NewStyle()
	dim := lipgloss.NewStyle().Foreground(muted)
	warn := lipgloss.NewStyle().Foreground(ColorWarning)
	fail := lipgloss.NewStyle().Foreground(ColorError)
	if !focused {
		userText = dim
		agentText = dim
		warn = dim
		fail = dim
	}

	var b strings.Builder
	lastKind := ""

	for _, log := range logs {
		if log.LineType == "pending_tool" || log.LineType == "pr_done_marker" {
			continue
		}

		switch log.LineType {
		case "user":
			if lastKind != "" {
				b.WriteString("\n")
			}
			for _, line := range wrapText(log.Content, bodyWidth-2) {
				b.WriteString(userBar.Render(Icon("▌", "|")) + " " + userText.Render(line) + "\n")
			}
			b.WriteString("\n")

		case "text":
			if lastKind == "tool" || lastKind == "system" {
				b.WriteString("\n")
			}
			// Replies are markdown — code fences, lists, emphasis — so they go
			// through the same renderer the task description uses. Falling back
			// to wrapped plain text keeps a malformed reply readable instead of
			// blank.
			rendered := ""
			if markdown != nil && focused {
				if out, err := markdown(log.Content); err == nil {
					rendered = trimRenderedMarkdown(out)
				}
			}
			if rendered != "" {
				b.WriteString(rendered + "\n\n")
			} else {
				for _, line := range wrapText(log.Content, bodyWidth) {
					b.WriteString(agentText.Render(line) + "\n")
				}
				b.WriteString("\n")
			}

		case "question":
			for _, line := range wrapText(log.Content, bodyWidth-2) {
				b.WriteString(warn.Render(Icon("? ", "? ")+line) + "\n")
			}
			b.WriteString("\n")

		case "image":
			b.WriteString(ImageLine(log.Content, bodyWidth) + "\n\n")

		case "error":
			b.WriteString(fail.Render(Icon("✗ ", "x ")+truncateLine(log.Content, bodyWidth-2)) + "\n")

		case "system":
			b.WriteString(dim.Render(Icon("· ", ". ")+truncateLine(log.Content, bodyWidth-2)) + "\n")

		default:
			// Commands already read as "$ ...". A second marker in front of them
			// is noise, so only unprefixed lines get a bullet.
			content := truncateLine(log.Content, bodyWidth-2)
			if strings.HasPrefix(content, "$ ") {
				b.WriteString(dim.Render("  "+content) + "\n")
			} else {
				b.WriteString(dim.Render(Icon("· ", ". ")+content) + "\n")
			}
		}

		lastKind = log.LineType
	}

	return b.String()
}

// RenderConversationForTest exposes the renderer so its output can be inspected
// without standing up a terminal.
func RenderConversationForTest(logs []*db.TaskLog, width int, focused bool) string {
	return renderConversation(logs, width, focused, nil)
}

// RenderConversationWithMarkdownForTest renders with a supplied markdown
// renderer so the formatting can be inspected without a terminal.
func RenderConversationWithMarkdownForTest(
	logs []*db.TaskLog,
	width int,
	focused bool,
	markdown func(string) (string, error),
) string {
	return renderConversation(logs, width, focused, markdown)
}

// trailingStyledSpaces matches the run of separately-styled spaces glamour pads
// each wrapped line with. They are invisible but real: they make every line the
// full pane width, so a short reply still paints edge to edge and the escape
// sequences dwarf the text.
var trailingStyledSpaces = regexp.MustCompile(`(?:\x1b\[[0-9;]*m *\x1b\[0m)+\s*$`)

func trimRenderedMarkdown(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		line = trailingStyledSpaces.ReplaceAllString(line, "")
		lines[i] = strings.TrimRight(line, " \t")
	}
	out := strings.Join(lines, "\n")
	return strings.Trim(out, "\n")
}

package bbstore

import (
	"testing"

	"github.com/bborn/bb-tui/internal/bbapi"
)

func TestLogLineForConversation(t *testing.T) {
	lineType, content, ok := LogLineFor(bbapi.TimelineRow{
		Kind: "conversation", Role: "user", Text: "please fix it",
	})
	if !ok || lineType != "user" || content != "please fix it" {
		t.Fatalf("user message mapped wrong: %q %q %v", lineType, content, ok)
	}

	lineType, _, ok = LogLineFor(bbapi.TimelineRow{
		Kind: "conversation", Role: "assistant", Text: "done",
	})
	if !ok || lineType != "text" {
		t.Fatalf("assistant message mapped wrong: %q %v", lineType, ok)
	}
}

func TestLogLineForSkipsEmptyRows(t *testing.T) {
	if _, _, ok := LogLineFor(bbapi.TimelineRow{Kind: "conversation", Text: "  "}); ok {
		t.Error("an empty message should be skipped, not rendered blank")
	}
	if _, _, ok := LogLineFor(bbapi.TimelineRow{Kind: "system"}); ok {
		t.Error("a system row with no title should be skipped")
	}
}

func TestLogLineForCommandCarriesItsExitCode(t *testing.T) {
	failed := 2
	lineType, content, ok := LogLineFor(bbapi.TimelineRow{
		Kind: "work", WorkKind: "command", Command: "go build ./...", ExitCode: &failed,
	})
	if !ok {
		t.Fatal("a command should render")
	}
	if lineType != "error" {
		t.Errorf("a non-zero exit should read as an error, got %q", lineType)
	}
	if content != "$ go build ./...  (exit 2)" {
		t.Errorf("content wrong: %q", content)
	}
}

func TestLogLineForCommandCollapsesWhitespace(t *testing.T) {
	zero := 0
	_, content, _ := LogLineFor(bbapi.TimelineRow{
		Kind: "work", WorkKind: "command", Command: "go  build\n  ./...", ExitCode: &zero,
	})
	if content != "$ go build ./..." {
		t.Fatalf("a multi-line command should collapse to one line, got %q", content)
	}
}

func TestLogLineForApprovalStates(t *testing.T) {
	cases := map[string]string{
		"pending": "write a file — waiting for you",
		"granted": "write a file — approved",
		"denied":  "write a file — denied",
	}
	for status, want := range cases {
		lineType, content, ok := LogLineFor(bbapi.TimelineRow{
			Kind: "work", WorkKind: "approval", Title: "write a file", Status: status,
		})
		if !ok || lineType != "question" {
			t.Fatalf("approval should be a question line, got %q %v", lineType, ok)
		}
		if content != want {
			t.Errorf("status %q: got %q, want %q", status, content, want)
		}
	}
}

func TestLogLineForFileChange(t *testing.T) {
	_, content, ok := LogLineFor(bbapi.TimelineRow{
		Kind: "work", WorkKind: "file-change", Path: "internal/ui/app.go",
	})
	if !ok || content != "edit internal/ui/app.go" {
		t.Fatalf("file change mapped wrong: %q %v", content, ok)
	}
}

func TestLogLineForUnknownKindIsSkipped(t *testing.T) {
	if _, _, ok := LogLineFor(bbapi.TimelineRow{Kind: "turn"}); ok {
		t.Error("a turn row carries no text of its own and should be skipped")
	}
}

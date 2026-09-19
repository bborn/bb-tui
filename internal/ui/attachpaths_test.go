package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempFile creates a file with a space in its name, which is what makes a
// dropped path interesting: the terminal escapes the space and the escape has
// to survive the round trip.
func writeTempFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func escapeForDrop(path string) string {
	return strings.ReplaceAll(path, " ", `\ `)
}

func TestExtractAttachmentsUnescapesADroppedPath(t *testing.T) {
	path := writeTempFile(t, "Screen Shot at 6.35.29 AM@2x.png")

	text, found := ExtractAttachments("look at "+escapeForDrop(path), 0)

	if len(found) != 1 {
		t.Fatalf("expected 1 attachment, got %d (text %q)", len(found), text)
	}
	if found[0].Path != path {
		t.Fatalf("path wrong:\n got %q\nwant %q", found[0].Path, path)
	}
	if !found[0].Image {
		t.Error("a .png should be recognised as an image")
	}
	if text != "look at [Image #1]" {
		t.Fatalf("placeholder wrong: %q", text)
	}
}

func TestExtractAttachmentsNumbersFromAnOffset(t *testing.T) {
	path := writeTempFile(t, "second.png")

	text, found := ExtractAttachments(escapeForDrop(path), 2)

	if len(found) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(found))
	}
	if text != "[Image #3]" {
		t.Fatalf("expected numbering to continue from the staged count, got %q", text)
	}
}

func TestExtractAttachmentsDistinguishesFilesFromImages(t *testing.T) {
	path := writeTempFile(t, "notes.md")

	text, found := ExtractAttachments(escapeForDrop(path), 0)

	if len(found) != 1 || found[0].Image {
		t.Fatalf("a .md should attach as a file, got %+v", found)
	}
	if text != "[File #1]" {
		t.Fatalf("placeholder wrong: %q", text)
	}
}

func TestExtractAttachmentsLeavesProseAlone(t *testing.T) {
	cases := []string{
		"see internal/ui/composer.go for the details",
		"/nope/definitely/missing.png",
		"ratio is 3/4 and that is fine",
		"",
	}

	for _, input := range cases {
		text, found := ExtractAttachments(input, 0)
		if len(found) != 0 {
			t.Errorf("%q should attach nothing, got %+v", input, found)
		}
		if text != input {
			t.Errorf("%q should be unchanged, got %q", input, text)
		}
	}
}

func TestExtractAttachmentsIgnoresADirectory(t *testing.T) {
	dir := t.TempDir()

	text, found := ExtractAttachments(dir, 0)

	if len(found) != 0 {
		t.Fatalf("a directory is not an attachment, got %+v", found)
	}
	if text != dir {
		t.Fatalf("text should be unchanged, got %q", text)
	}
}

func TestExtractAttachmentsHandlesQuotedPaths(t *testing.T) {
	path := writeTempFile(t, "quoted name.png")

	text, found := ExtractAttachments(`"`+path+`"`, 0)

	if len(found) != 1 {
		t.Fatalf("expected 1 attachment, got %d (text %q)", len(found), text)
	}
	if found[0].Path != path {
		t.Fatalf("quotes should be stripped:\n got %q\nwant %q", found[0].Path, path)
	}
}

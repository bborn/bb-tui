package ui

import "testing"

func TestExtractAttachmentsFromDroppedPath(t *testing.T) {
	real := "/Users/bruno/Library/Application Support/CleanShot/media/media_uImdFZGmmp/CleanShot 2026-09-19 at 6.35.29 AM@2x.png"
	escaped := `/Users/bruno/Library/Application\ Support/CleanShot/media/media_uImdFZGmmp/CleanShot\ 2026-09-19\ at\ 6.35.29\ AM@2x.png`

	text, found := ExtractAttachments("look at "+escaped, 0)
	if len(found) != 1 {
		t.Fatalf("expected 1 attachment, got %d (text %q)", len(found), text)
	}
	if found[0].Path != real {
		t.Fatalf("unescaped path wrong:\n got %q\nwant %q", found[0].Path, real)
	}
	if !found[0].Image {
		t.Fatal("png should be recognised as an image")
	}
	if text != "look at [Image #1]" {
		t.Fatalf("placeholder wrong: %q", text)
	}
}

func TestExtractAttachmentsIgnoresProse(t *testing.T) {
	text, found := ExtractAttachments("see internal/ui/composer.go and /nope/missing.png", 0)
	if len(found) != 0 {
		t.Fatalf("expected no attachments, got %d", len(found))
	}
	if text != "see internal/ui/composer.go and /nope/missing.png" {
		t.Fatalf("text should be untouched, got %q", text)
	}
}

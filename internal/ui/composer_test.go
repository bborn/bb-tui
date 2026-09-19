package ui

import "testing"

func TestPrettyPermissionUsesTheAppsWording(t *testing.T) {
	cases := map[string]string{
		"full":          "Full Access",
		"auto":          "Auto",
		"accept-edits":  "Accept Edits",
		"default":       "Ask",
		"":              "",
		"something-new": "something-new",
	}
	for mode, want := range cases {
		if got := prettyPermission(mode); got != want {
			t.Errorf("prettyPermission(%q) = %q, want %q", mode, got, want)
		}
	}
}

func TestLastTokenFindsTheWordUnderTheCursor(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"/dep":            "/dep",
		"look at @comp":   "@comp",
		"look at @comp ":  "",
		"trailing\n":      "",
		"multi word here": "here",
	}
	for input, want := range cases {
		if got := lastToken(input); got != want {
			t.Errorf("lastToken(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSlashMenuRanksPrefixMatchesFirst(t *testing.T) {
	menu := slashMenu{
		items: []SlashItem{
			{Name: "design-shotgun"},
			{Name: "deslop"},
			{Name: "code-deslop"},
			{Name: "unrelated"},
		},
		query: "des",
	}
	menu.refilter()

	if len(menu.filtered) != 3 {
		t.Fatalf("expected 3 matches, got %d: %+v", len(menu.filtered), menu.filtered)
	}
	if menu.filtered[0].Name != "deslop" {
		t.Errorf("shortest prefix match should come first, got %q", menu.filtered[0].Name)
	}
	for _, item := range menu.filtered {
		if item.Name == "unrelated" {
			t.Error("a non-matching item should be filtered out")
		}
	}
}

func TestSlashMenuEmptyQueryKeepsEverything(t *testing.T) {
	menu := slashMenu{items: []SlashItem{{Name: "a"}, {Name: "b"}}}
	menu.refilter()
	if len(menu.filtered) != 2 {
		t.Fatalf("an empty query should match everything, got %d", len(menu.filtered))
	}
}

func TestSlashMenuWrapsWhenMoving(t *testing.T) {
	menu := slashMenu{items: []SlashItem{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	menu.refilter()

	menu.move(-1)
	if menu.index != 2 {
		t.Errorf("moving up from the first item should wrap to the last, got %d", menu.index)
	}
	menu.move(1)
	if menu.index != 0 {
		t.Errorf("moving down from the last item should wrap to the first, got %d", menu.index)
	}
}

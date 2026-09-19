package ui

import "testing"

func TestShortProjectName(t *testing.T) {
	cases := []struct {
		project string
		want    string
	}{
		{"bb", "bb"},
		{"Personal", "Personal"},
		{"my-side-project", "msp"},
		{"acme_web_app", "awa"},
		{"some long name", "sln"},
		{"averyverylongsingleword", "averyver"},
		{"", ""},
		{"  ", ""},
		{"exactly8", "exactly8"},
		{"a/b/c/d/e/f/g/h/i", "abcdefghi"},
	}

	for _, test := range cases {
		if got := ShortProjectName(test.project); got != test.want {
			t.Errorf("ShortProjectName(%q) = %q, want %q", test.project, got, test.want)
		}
	}
}

func TestShortProjectNameNeverExceedsTheLimitForSingleWords(t *testing.T) {
	got := ShortProjectName("supercalifragilistic")
	if len([]rune(got)) > projectAbbreviationLimit {
		t.Fatalf("abbreviation %q is longer than the limit", got)
	}
}

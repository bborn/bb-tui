package ui

import "testing"

func TestDetectImageSupport(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want TerminalImageSupport
	}{
		{"iterm", map[string]string{"TERM_PROGRAM": "iTerm.app"}, ImagesITerm2},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, ImagesITerm2},
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"}, ImagesKitty},
		{"ghostty", map[string]string{"GHOSTTY_RESOURCES_DIR": "/x"}, ImagesKitty},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, ImagesUnsupported},
		{"apple terminal", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, ImagesUnsupported},
		{"iterm inside tmux", map[string]string{"TERM_PROGRAM": "iTerm.app", "TMUX": "/tmp/x"}, ImagesUnsupported},
		{"tmux with passthrough", map[string]string{"TERM_PROGRAM": "iTerm.app", "TMUX": "/tmp/x", "BB_TUI_TMUX_PASSTHROUGH": "1"}, ImagesITerm2},
		{"opt out", map[string]string{"TERM_PROGRAM": "iTerm.app", "BB_TUI_NO_IMAGES": "1"}, ImagesUnsupported},
	}

	for _, test := range cases {
		got := detectImageSupport(func(key string) string { return test.env[key] })
		if got != test.want {
			t.Errorf("%s: got %v want %v", test.name, got, test.want)
		}
	}
}

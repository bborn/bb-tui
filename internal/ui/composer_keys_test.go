package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComposerOwnsMacEditingKeys(t *testing.T) {
	owned := []tea.KeyMsg{
		{Type: tea.KeyBackspace, Alt: true},      // option+delete: delete word
		{Type: tea.KeyDelete, Alt: true},         // option+fn+delete
		{Type: tea.KeyLeft, Alt: true},           // option+left: word left
		{Type: tea.KeyRight, Alt: true},          // option+right: word right
		{Type: tea.KeyCtrlU},                     // cmd+delete, via CSI translation
		{Type: tea.KeyCtrlA},                     // cmd+left
		{Type: tea.KeyCtrlE},                     // cmd+right
		{Type: tea.KeyCtrlW},                     // delete word back
		{Type: tea.KeyRunes, Runes: []rune{'o'}}, // a letter is text, not "open"
	}
	for _, key := range owned {
		if !composerOwnsKey(key) {
			t.Errorf("composer should own %v (%s)", key.Type, key.String())
		}
	}

	notOwned := []tea.KeyMsg{
		{Type: tea.KeyUp},
		{Type: tea.KeyDown},
		{Type: tea.KeyPgUp},
		{Type: tea.KeyRunes, Runes: []rune{'o'}, Alt: true}, // alt+o is a shortcut
	}
	for _, key := range notOwned {
		if composerOwnsKey(key) {
			t.Errorf("composer should not own %v (%s)", key.Type, key.String())
		}
	}
}

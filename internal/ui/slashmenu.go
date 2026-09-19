package ui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const slashMenuRows = 6

// SlashItem is one thing "/" can insert.
type SlashItem struct {
	Name        string
	Description string
	Scope       string
}

// MenuKind is which trigger opened the completion menu.
type MenuKind string

const (
	// MenuSkills is "/", which bb declares in each provider's composerActions.
	MenuSkills MenuKind = "skills"
	// MenuMentions is "@", which the app offers for files, folders, sections
	// and threads.
	MenuMentions MenuKind = "mentions"
	// MenuModels picks which model answers the next message.
	MenuModels MenuKind = "models"
	// MenuPermissions picks how much that model is allowed to do.
	MenuPermissions MenuKind = "permissions"
)

// MenuItemsHook supplies completions for a trigger. The query is passed so a
// large project can be filtered by the server rather than shipped whole.
var MenuItemsHook func(taskID int64, kind MenuKind, query string) []SlashItem

type slashMenu struct {
	kind     MenuKind
	open     bool
	query    string
	items    []SlashItem
	filtered []SlashItem
	index    int
}

// score ranks a candidate against what has been typed. A prefix match beats a
// word-start match, which beats a substring, so "pl" offers "plan" before
// "duplicate-check".
func score(name, query string) int {
	if query == "" {
		return 1
	}
	name = strings.ToLower(name)
	query = strings.ToLower(query)
	switch {
	case strings.HasPrefix(name, query):
		return 3
	case strings.Contains("-"+name, "-"+query):
		return 2
	case strings.Contains(name, query):
		return 1
	}
	return 0
}

func (m *slashMenu) refilter() {
	m.filtered = m.filtered[:0]
	type scored struct {
		item  SlashItem
		score int
	}
	var ranked []scored
	for _, item := range m.items {
		if value := score(item.Name, m.query); value > 0 {
			ranked = append(ranked, scored{item, value})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return len(ranked[i].item.Name) < len(ranked[j].item.Name)
	})
	for _, entry := range ranked {
		m.filtered = append(m.filtered, entry.item)
	}
	if m.index >= len(m.filtered) {
		m.index = maxInt(0, len(m.filtered)-1)
	}
}

func (m *slashMenu) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.index += delta
	if m.index < 0 {
		m.index = len(m.filtered) - 1
	}
	if m.index >= len(m.filtered) {
		m.index = 0
	}
}

func (m *slashMenu) selected() (SlashItem, bool) {
	if m.index < 0 || m.index >= len(m.filtered) {
		return SlashItem{}, false
	}
	return m.filtered[m.index], true
}

// render draws the menu above the input, newest convention for a completion
// popup in a terminal: a short window, the match count, and no scrollbar.
func (m *slashMenu) render(width int) string {
	if !m.open {
		return ""
	}

	trigger := "/"
	empty := "no matching skills"
	switch m.kind {
	case MenuMentions:
		trigger = "@"
		empty = "no matching files"
	case MenuModels:
		trigger = "model "
		empty = "no models offered for this provider"
	case MenuPermissions:
		trigger = "permission "
		empty = "no permission modes offered"
	}
	title := lipgloss.NewStyle().Foreground(ColorMuted).Render("  " + trigger + m.query)
	if len(m.filtered) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, title,
			lipgloss.NewStyle().Foreground(ColorMuted).Render("    "+empty))
	}

	start := 0
	if m.index >= slashMenuRows {
		start = m.index - slashMenuRows + 1
	}
	end := minInt(len(m.filtered), start+slashMenuRows)

	lines := []string{title}
	for i := start; i < end; i++ {
		item := m.filtered[i]
		marker := "  "
		nameStyle := lipgloss.NewStyle()
		if i == m.index {
			marker = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("> ")
			nameStyle = nameStyle.Foreground(ColorPrimary).Bold(true)
		}
		name := nameStyle.Render(item.Name)
		room := width - lipgloss.Width(name) - 8
		description := ""
		if room > 12 && item.Description != "" {
			description = lipgloss.NewStyle().Foreground(ColorMuted).
				Render("  " + truncateLine(item.Description, room))
		}
		lines = append(lines, "  "+marker+name+description)
	}

	if len(m.filtered) > end-start {
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorMuted).
			Render("    "+Icon("↓", "v")+" "+strconv.Itoa(len(m.filtered)-end)+" more"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *slashMenu) height() int {
	if !m.open {
		return 0
	}
	if len(m.filtered) == 0 {
		return 2
	}
	rows := minInt(len(m.filtered), slashMenuRows)
	if len(m.filtered) > rows {
		rows++
	}
	return rows + 1
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

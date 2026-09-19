package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bborn/bb-tui/internal/db"
	"github.com/bborn/bb-tui/internal/executor"
	"github.com/bborn/bb-tui/internal/ui"
)

// parseKeyEvents turns "down,enter,hello" into key messages. Anything that is
// not a named key is typed rune by rune, so a whole message can be sent in one
// argument. Ported from TaskYou's debug harness.
func parseKeyEvents(input string) []tea.Msg {
	var msgs []tea.Msg
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		var msg tea.KeyMsg
		switch strings.ToLower(part) {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "shift+enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
		case "esc", "escape":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "delete":
			msg = tea.KeyMsg{Type: tea.KeyDelete}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "pgup", "pageup":
			msg = tea.KeyMsg{Type: tea.KeyPgUp}
		case "pgdown", "pagedown":
			msg = tea.KeyMsg{Type: tea.KeyPgDown}
		case "home":
			msg = tea.KeyMsg{Type: tea.KeyHome}
		case "end":
			msg = tea.KeyMsg{Type: tea.KeyEnd}
		case "shift+up":
			msg = tea.KeyMsg{Type: tea.KeyShiftUp}
		case "shift+down":
			msg = tea.KeyMsg{Type: tea.KeyShiftDown}
		case "ctrl+up":
			msg = tea.KeyMsg{Type: tea.KeyCtrlUp}
		case "ctrl+down":
			msg = tea.KeyMsg{Type: tea.KeyCtrlDown}
		case "ctrl+g":
			msg = tea.KeyMsg{Type: tea.KeyCtrlG}
		case "ctrl+t":
			msg = tea.KeyMsg{Type: tea.KeyCtrlT}
		case "ctrl+l":
			msg = tea.KeyMsg{Type: tea.KeyCtrlL}
		case "ctrl+c":
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		case "ctrl+p":
			msg = tea.KeyMsg{Type: tea.KeyCtrlP}
		case "ctrl+k":
			msg = tea.KeyMsg{Type: tea.KeyCtrlK}
		case "ctrl+n":
			msg = tea.KeyMsg{Type: tea.KeyCtrlN}
		default:
			// "click:XxY" sends a left-button release at those cells, so mouse
			// behaviour can be checked without a terminal. The separator is "x"
			// rather than a comma because the key list is itself comma-split.
			if strings.HasPrefix(strings.ToLower(part), "click:") {
				coords := strings.SplitN(strings.TrimPrefix(strings.ToLower(part), "click:"), "x", 2)
				if len(coords) == 2 {
					x, errX := strconv.Atoi(strings.TrimSpace(coords[0]))
					y, errY := strconv.Atoi(strings.TrimSpace(coords[1]))
					if errX == nil && errY == nil {
						msgs = append(msgs, tea.MouseMsg{
							X:      x,
							Y:      y,
							Action: tea.MouseActionRelease,
							Button: tea.MouseButtonLeft,
						})
						continue
					}
				}
			}
			for _, r := range part {
				msgs = append(msgs, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

func drainOne(model *ui.AppModel, cmd tea.Cmd) {
	for depth := 0; cmd != nil && depth < 8; depth++ {
		msg := cmd()
		if msg == nil {
			return
		}
		if _, ok := msg.(tea.BatchMsg); ok {
			return
		}
		_, cmd = model.Update(msg)
	}
}

// runDebugState drives the real model headlessly and prints what it would show.
// This is how bb-tui is checked without a terminal: the same model, the same
// keys, and a view tree that can be diffed.
func runDebugState(database *db.DB, exec *executor.Executor, keys string, dumpView bool) {
	model := ui.NewAppModel(database, exec, ".", "0.0.1")
	model.Update(tea.WindowSizeMsg{Width: 160, Height: 45})

	tasks, err := database.ListTasks(db.ListTasksOptions{IncludeClosed: true, Limit: 1000})
	if err == nil {
		model.SetTasks(tasks)
	}
	model.Update(tea.WindowSizeMsg{Width: 160, Height: 45})

	// bubbletea runs the commands a model returns and feeds their messages back.
	// Without that loop a key that only schedules work — opening a thread, say —
	// appears to do nothing, so the harness drains them the way the runtime does.
	drain := func(cmd tea.Cmd) {
		for depth := 0; cmd != nil && depth < 12; depth++ {
			msg := cmd()
			if msg == nil {
				return
			}
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, inner := range batch {
					drainOne(model, inner)
				}
				return
			}
			_, cmd = model.Update(msg)
		}
	}

	for _, msg := range parseKeyEvents(keys) {
		_, cmd := model.Update(msg)
		drain(cmd)
		time.Sleep(20 * time.Millisecond)

		// Opening a thread only requests its conversation; without waiting for
		// it, every scroll check would be scrolling an empty pane.
		if model.DebugDetailLogCount() == 0 {
			for attempt := 0; attempt < 20; attempt++ {
				time.Sleep(150 * time.Millisecond)
				drain(model.DebugReloadDetail())
				if model.DebugDetailLogCount() > 0 {
					break
				}
			}
		}
	}

	if dumpView {
		fmt.Println(model.View())
		return
	}

	state := model.GenerateDebugState()
	data, _ := json.MarshalIndent(state, "", "  ")
	fmt.Println(string(data))
	_ = os.Stdout.Sync()
}

package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bborn/bb-tui/internal/db"
)

// ApprovalChoice is one answer the user can give to a pending interaction.
type ApprovalChoice struct {
	Label       string
	Description string
	// Decision is set for tool approvals: allow_once, allow_for_session, deny.
	Decision string
	// QuestionID and OptionValue are set for user questions.
	QuestionID  string
	OptionValue string
}

// PendingApproval is one thing a thread is waiting on, flattened for display.
type PendingApproval struct {
	ID      string
	Kind    string
	Summary string
	Prompt  string
	Choices []ApprovalChoice
}

// ApprovalsHook returns whatever the task's thread is waiting on.
var ApprovalsHook func(taskID int64) ([]PendingApproval, error)

// AnswerApprovalHook delivers the chosen answer.
var AnswerApprovalHook func(taskID int64, approval PendingApproval, choice ApprovalChoice) error

// ApprovalsModel is a modal that answers whatever a thread is blocked on. It
// follows ActionPickerModel: a self-contained sub-model with its own View,
// reached through a dedicated View constant so it never touches the huh router.
type ApprovalsModel struct {
	taskTitle string
	approvals []PendingApproval
	index     int
	choice    int
	width     int
	height    int
	err       string

	answered  *ApprovalChoice
	cancelled bool
}

func NewApprovalsModel(taskTitle string, approvals []PendingApproval, width, height int) *ApprovalsModel {
	return &ApprovalsModel{
		taskTitle: taskTitle,
		approvals: approvals,
		width:     width,
		height:    height,
	}
}

func (m *ApprovalsModel) Init() tea.Cmd { return nil }

func (m *ApprovalsModel) current() *PendingApproval {
	if m.index < 0 || m.index >= len(m.approvals) {
		return nil
	}
	return &m.approvals[m.index]
}

func (m *ApprovalsModel) Update(msg tea.Msg) (*ApprovalsModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	approval := m.current()
	if approval == nil {
		m.cancelled = true
		return m, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		m.cancelled = true
	case "tab":
		if len(m.approvals) > 1 {
			m.index = (m.index + 1) % len(m.approvals)
			m.choice = 0
		}
	case "enter":
		if m.choice >= 0 && m.choice < len(approval.Choices) {
			selected := approval.Choices[m.choice]
			m.answered = &selected
		}
	case "up", "k", "ctrl+p":
		if len(approval.Choices) > 0 {
			m.choice--
			if m.choice < 0 {
				m.choice = len(approval.Choices) - 1
			}
		}
	case "down", "j", "ctrl+n":
		if len(approval.Choices) > 0 {
			m.choice++
			if m.choice >= len(approval.Choices) {
				m.choice = 0
			}
		}
	}
	return m, nil
}

func (m *ApprovalsModel) View() string {
	modalWidth := min(84, m.width-4)
	approval := m.current()

	title := "Waiting on you"
	if len(m.approvals) > 1 {
		title = fmt.Sprintf("Waiting on you (%d of %d)", m.index+1, len(m.approvals))
	}
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWarning).
		Render(title)

	subtitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginBottom(1).
		Render(truncateLine(m.taskTitle, modalWidth-6))

	var body strings.Builder
	if approval == nil {
		body.WriteString(lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Render("Nothing is waiting on you in this thread."))
	} else {
		prompt := approval.Prompt
		if strings.TrimSpace(prompt) == "" {
			prompt = approval.Summary
		}
		body.WriteString(lipgloss.NewStyle().Render(wrapLine(prompt, modalWidth-6)))
		body.WriteString("\n\n")
		for i, choice := range approval.Choices {
			body.WriteString(m.renderChoice(choice, i == m.choice, modalWidth-6))
			if i < len(approval.Choices)-1 {
				body.WriteString("\n")
			}
		}
	}

	if m.err != "" {
		body.WriteString("\n\n")
		body.WriteString(lipgloss.NewStyle().Foreground(ColorError).Render(m.err))
	}

	hint := "enter: answer  esc: cancel  " + IconArrowUp() + "/" + IconArrowDown() + ": choose"
	if len(m.approvals) > 1 {
		hint += "  tab: next"
	}
	help := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render(hint)

	content := lipgloss.JoinVertical(lipgloss.Left, header, subtitle, body.String(), help)

	modalBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorWarning).
		Padding(1, 2).
		Width(modalWidth)

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(modalBox.Render(content))
}

func (m *ApprovalsModel) renderChoice(choice ApprovalChoice, selected bool, width int) string {
	var line strings.Builder
	if selected {
		line.WriteString(lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("> "))
	} else {
		line.WriteString("  ")
	}

	labelStyle := lipgloss.NewStyle()
	if selected {
		labelStyle = labelStyle.Bold(true).Foreground(ColorPrimary)
	}
	line.WriteString(labelStyle.Render(truncateLine(choice.Label, width-4)))

	if choice.Description != "" {
		line.WriteString("\n    ")
		line.WriteString(lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render(truncateLine(choice.Description, width-6)))
	}
	return line.String()
}

func (m *ApprovalsModel) Answered() (*PendingApproval, *ApprovalChoice) {
	if m.answered == nil {
		return nil, nil
	}
	return m.current(), m.answered
}

func (m *ApprovalsModel) ClearAnswer(message string) {
	m.answered = nil
	m.err = message
}

func (m *ApprovalsModel) IsCancelled() bool { return m.cancelled }

func (m *ApprovalsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func truncateLine(value string, width int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if width <= 1 || len(value) <= width {
		return value
	}
	return value[:width-1] + Icon("…", "...")
}

func wrapLine(value string, width int) string {
	if width <= 8 {
		return value
	}
	var out strings.Builder
	for i, paragraph := range strings.Split(value, "\n") {
		if i > 0 {
			out.WriteString("\n")
		}
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) <= width {
				line += " " + word
				continue
			}
			out.WriteString(line + "\n")
			line = word
		}
		out.WriteString(line)
	}
	return out.String()
}

// openApprovals fetches what the task is waiting on and shows the modal. It is
// a no-op when nothing is pending, so the key is safe to press on any task.
func (m *AppModel) openApprovals(task *db.Task) (tea.Model, tea.Cmd) {
	if ApprovalsHook == nil || task == nil {
		return m, nil
	}
	approvals, err := ApprovalsHook(task.ID)
	if err != nil {
		m.setBanner(IconBlocked() + " could not read what this thread is waiting on: " + err.Error())
		return m, nil
	}
	if len(approvals) == 0 {
		m.setBanner("nothing is waiting on you in this thread")
		return m, nil
	}
	m.approvalsTask = task
	m.approvalsView = NewApprovalsModel(task.Title, approvals, m.width, m.height)
	m.previousView = m.currentView
	m.currentView = ViewApprovals
	return m, m.approvalsView.Init()
}

func (m *AppModel) updateApprovals(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.approvalsView == nil {
		return m, nil
	}

	var cmd tea.Cmd
	m.approvalsView, cmd = m.approvalsView.Update(msg)

	if m.approvalsView.IsCancelled() {
		m.approvalsView = nil
		m.approvalsTask = nil
		m.currentView = m.previousView
		return m, nil
	}

	approval, choice := m.approvalsView.Answered()
	if approval == nil || choice == nil {
		return m, cmd
	}

	task := m.approvalsTask
	if AnswerApprovalHook == nil || task == nil {
		m.approvalsView = nil
		m.approvalsTask = nil
		m.currentView = m.previousView
		return m, nil
	}

	if err := AnswerApprovalHook(task.ID, *approval, *choice); err != nil {
		m.approvalsView.ClearAnswer(err.Error())
		return m, cmd
	}

	m.approvalsView = nil
	m.approvalsTask = nil
	m.currentView = m.previousView
	m.setBanner(IconDone() + " answered: " + choice.Label)
	return m, m.loadTasks()
}

func (m *AppModel) setBanner(text string) {
	m.notification = text
	m.notifyUntil = time.Now().Add(6 * time.Second)
}

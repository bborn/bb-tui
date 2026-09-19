package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bborn/bb-tui/internal/bbapi"
	"github.com/bborn/bb-tui/internal/bbstore"
	"github.com/bborn/bb-tui/internal/config"
	"github.com/bborn/bb-tui/internal/db"
	"github.com/bborn/bb-tui/internal/executor"
	"github.com/bborn/bb-tui/internal/ui"
)

func localStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".bb-tui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "bb-tui.db"), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bb-tui:", err)
	os.Exit(1)
}

func providerFor(executorName string) string {
	switch strings.TrimSpace(strings.ToLower(executorName)) {
	case "claude", "claude-code":
		return "claude-code"
	case "codex":
		return "codex"
	case "pi":
		return "pi"
	case "cursor":
		return "acp-cursor"
	case "opencode":
		return "acp-opencode"
	}
	return ""
}

func permissionFor(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "dangerous":
		return "full"
	case "accept-edits", "auto":
		return "auto"
	case "default":
		return "default"
	}
	return ""
}

func toApproval(interaction bbapi.Interaction) ui.PendingApproval {
	approval := ui.PendingApproval{
		ID:      interaction.ID,
		Kind:    interaction.Payload.Kind,
		Summary: interaction.Summary(),
		Prompt:  interaction.Summary(),
	}

	switch interaction.Payload.Kind {
	case "approval":
		approval.Choices = []ui.ApprovalChoice{
			{Label: "Approve once", Decision: "allow_once"},
			{Label: "Approve for the rest of this session", Decision: "allow_for_session"},
			{Label: "Deny", Decision: "deny"},
		}

	case "user_question":
		for _, question := range interaction.Payload.Questions {
			if question.Prompt != "" {
				approval.Prompt = question.Prompt
			}
			for _, option := range question.Options {
				approval.Choices = append(approval.Choices, ui.ApprovalChoice{
					Label:       option.Label,
					Description: option.Description,
					QuestionID:  question.ID,
					OptionValue: option.Value,
				})
			}
			break
		}
	}

	return approval
}

func installHooks(client *bbapi.Client, store *bbstore.Store) {
	ui.DisableUpstreamVersionCheck = true
	ui.ColumnTitles = [4]string{"Queued", "Running", "Waiting on you", "Idle"}
	ui.ComposerEnabled = true
	ui.ConversationView = true
	ui.ActivityHook = store.Activity
	ui.ContextHook = func(taskID int64) ui.ComposerContext {
		context := store.Context(taskID)
		return ui.ComposerContext{
			Project:     context.Project,
			Environment: context.Environment,
			Provider:    context.Provider,
			Model:       context.Model,
			Reasoning:   context.Reasoning,
			Permission:  context.Permission,
		}
	}
	ui.StatusChoices = func() []ui.StatusChoice {
		return []ui.StatusChoice{
			{Value: db.StatusDone, Label: "Stop the run"},
			{Value: db.StatusArchived, Label: "Archive"},
		}
	}

	thread := store.ThreadID

	ui.SendPromptHook = func(taskID int64, text string, attachments []ui.Attachment, model, permission string) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		files := make([]bbapi.LocalAttachment, 0, len(attachments))
		for _, attachment := range attachments {
			files = append(files, bbapi.LocalAttachment{
				Path:  attachment.Path,
				Image: attachment.Image,
			})
		}
		if err := client.Send(id, text, files, model, permission); err != nil {
			return err
		}
		store.RefreshOpenLogs()
		return nil
	}

	ui.RetryTurnHook = func(taskID int64) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		if err := client.Retry(id); err != nil {
			return err
		}
		_ = store.Refresh()
		return nil
	}

	ui.StopTaskHook = func(taskID int64) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		return client.Stop(id)
	}

	ui.PinHook = func(taskID int64, pinned bool) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		if pinned {
			err = client.Pin(id)
		} else {
			err = client.Unpin(id)
		}
		if err == nil {
			_ = store.Refresh()
		}
		return err
	}

	ui.ArchiveHook = func(taskID int64) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		if err := client.Archive(id); err != nil {
			return err
		}
		return store.Refresh()
	}

	ui.DeleteHook = func(taskID int64) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		if err := client.Delete(id); err != nil {
			return err
		}
		return store.Refresh()
	}

	ui.StatusHook = func(taskID int64, status string) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		switch status {
		case db.StatusArchived:
			err = client.Archive(id)
		case db.StatusDone:
			err = client.Stop(id)
		default:
			return fmt.Errorf("bb has no equivalent for status %q — threads move themselves", status)
		}
		if err == nil {
			_ = store.Refresh()
		}
		return err
	}

	ui.CreateTaskHook = func(task *db.Task) (int64, error) {
		projectID, err := store.ProjectIDForName(task.Project)
		if err != nil {
			return 0, err
		}
		prompt := strings.TrimSpace(task.Body)
		if prompt == "" {
			prompt = strings.TrimSpace(task.Title)
		}
		if prompt == "" {
			return 0, fmt.Errorf("a bb thread needs a prompt — put it in the description")
		}
		threadID, err := client.Spawn(bbapi.SpawnRequest{
			ProjectID:      projectID,
			Title:          task.Title,
			Prompt:         prompt,
			ProviderID:     providerFor(task.Executor),
			Model:          task.Model,
			PermissionMode: permissionFor(task.PermissionMode),
		})
		if err != nil {
			return 0, err
		}
		taskID := store.TaskID(threadID)
		_ = store.Refresh()
		return taskID, nil
	}

	ui.MenuItemsHook = func(taskID int64, kind ui.MenuKind, query string) []ui.SlashItem {
		switch kind {
		case ui.MenuSkills:
			skills := store.Skills(taskID)
			items := make([]ui.SlashItem, 0, len(skills))
			for _, skill := range skills {
				items = append(items, ui.SlashItem{
					Name:        skill.Name,
					Description: skill.Description,
					Scope:       skill.Scope,
				})
			}
			return items

		case ui.MenuModels:
			task, err := store.GetTask(taskID)
			if err != nil {
				return nil
			}
			options, err := client.Options(task.Executor)
			if err != nil {
				return nil
			}
			items := make([]ui.SlashItem, 0, len(options.Models))
			for _, model := range options.Models {
				items = append(items, ui.SlashItem{
					Name:        model.DisplayName,
					Description: model.Description,
					Scope:       model.ID,
				})
			}
			return items

		case ui.MenuPermissions:
			task, err := store.GetTask(taskID)
			if err != nil {
				return nil
			}
			options, err := client.Options(task.Executor)
			if err != nil {
				return nil
			}
			items := make([]ui.SlashItem, 0, len(options.PermissionModes))
			for _, mode := range options.PermissionModes {
				items = append(items, ui.SlashItem{
					Name:  ui.PrettyPermission(mode),
					Scope: mode,
				})
			}
			return items

		case ui.MenuMentions:
			files := store.SearchFiles(taskID, query, 40)
			items := make([]ui.SlashItem, 0, len(files))
			for _, file := range files {
				items = append(items, ui.SlashItem{Name: file.Path})
			}
			return items
		}
		return nil
	}

	ui.ApprovalsHook = func(taskID int64) ([]ui.PendingApproval, error) {
		id, err := thread(taskID)
		if err != nil {
			return nil, err
		}
		interactions, err := client.Interactions(id)
		if err != nil {
			return nil, err
		}
		approvals := make([]ui.PendingApproval, 0, len(interactions))
		for _, interaction := range interactions {
			approvals = append(approvals, toApproval(interaction))
		}
		return approvals, nil
	}

	ui.AnswerApprovalHook = func(taskID int64, approval ui.PendingApproval, choice ui.ApprovalChoice) error {
		id, err := thread(taskID)
		if err != nil {
			return err
		}
		switch approval.Kind {
		case "approval":
			err = client.ResolveApproval(id, approval.ID, choice.Decision)
		case "user_question":
			err = client.AnswerQuestion(id, approval.ID, map[string][]string{
				choice.QuestionID: {choice.OptionValue},
			})
		default:
			return fmt.Errorf("bb-tui cannot answer a %q interaction yet — use the bb app", approval.Kind)
		}
		if err == nil {
			_ = store.Refresh()
			store.RefreshOpenLogs()
		}
		return err
	}
}

func main() {
	demo := len(os.Args) > 1 && contains(os.Args, "--demo")

	path, err := localStatePath()
	if err != nil {
		fail(err)
	}
	if demo {
		// A fresh file per run: the board persists its filter, list mode and
		// saved views, and a screenshot should show the defaults rather than
		// whatever the last person left behind.
		path = filepath.Join(os.TempDir(), fmt.Sprintf("bb-tui-demo-%d.db", os.Getpid()))
		defer os.Remove(path)
	}

	// --demo runs against invented data so the screenshots in this repository can
	// be regenerated by anyone, without a bb server and without anyone's threads.
	var client *bbapi.Client
	if !demo {
		client = bbapi.New("")
		if err := client.Health(); err != nil {
			fail(fmt.Errorf("%w\n\nIs bb running? Start the bb app, or set BB_SERVER_URL", err))
		}
	}

	db.DefaultSavedViews([]db.SavedView{
		{Name: "Waiting on you", Query: "status:waiting", SortOrder: 1},
		{Name: "Running", Query: "status:running", SortOrder: 2},
		{Name: "Pinned", Query: "is:pinned", SortOrder: 3},
		{Name: "In review", Query: "has:pr", SortOrder: 4},
	})

	database, err := db.Open(path)
	if err != nil {
		fail(err)
	}
	defer database.Close()

	if demo {
		runDemo(database)
		return
	}

	store := bbstore.New(client)
	if err := store.Refresh(); err != nil {
		fail(err)
	}
	if err := database.PurgeBackedTables(); err != nil {
		fail(err)
	}
	database.SetBackend(store)

	if err := database.CompleteOnboarding(); err != nil {
		fail(err)
	}

	installHooks(client, store)

	exec := executor.New(database, config.New(database))

	if len(os.Args) > 1 && os.Args[1] == "--debug-state" {
		keys := ""
		dumpView := false
		for i, arg := range os.Args {
			if arg == "--keys" && i+1 < len(os.Args) {
				keys = os.Args[i+1]
			}
			if arg == "--view" {
				dumpView = true
			}
		}
		runDebugState(database, exec, keys, dumpView)
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "--sync-only" {
		tasks, listErr := database.ListTasks(db.ListTasksOptions{})
		if listErr != nil {
			fail(listErr)
		}
		fmt.Printf("%d tasks from %s\n", len(tasks), client.BaseURL)
		for i, task := range tasks {
			if i >= 5 {
				break
			}
			fmt.Printf("  #%d %-11s [%s] %s\n", task.ID, task.Status, task.Project, task.Title)
		}
		return
	}

	model := ui.NewAppModel(database, exec, ".", "0.0.1")
	model.SetMouseCaptured(true)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	stop := make(chan struct{})
	go store.Watch(stop, func() {
		program.Send(struct{}{})
	})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = store.Refresh()
			case <-stop:
				return
			}
		}
	}()
	defer close(stop)

	if _, err := program.Run(); err != nil {
		fail(err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// runDemo installs the invented workspace and either renders a frame or opens
// the TUI on it.
func runDemo(database *db.DB) {
	demo := bbstore.NewDemoStore()
	database.SetBackend(demo)
	_ = database.CompleteOnboarding()

	ui.DisableUpstreamVersionCheck = true
	ui.ColumnTitles = [4]string{"Queued", "Running", "Waiting on you", "Idle"}
	ui.ComposerEnabled = true
	ui.ConversationView = true
	ui.ActivityHook = demo.Activity
	ui.ContextHook = func(taskID int64) ui.ComposerContext {
		context := demo.Context(taskID)
		return ui.ComposerContext{
			Project:     context.Project,
			Environment: context.Environment,
			Provider:    context.Provider,
			Model:       context.Model,
			Reasoning:   context.Reasoning,
			Permission:  context.Permission,
		}
	}
	ui.MenuItemsHook = func(_ int64, kind ui.MenuKind, _ string) []ui.SlashItem {
		switch kind {
		case ui.MenuSkills:
			return []ui.SlashItem{
				{Name: "deploy", Description: "Prepare a production deploy: release notes, migrations, flags"},
				{Name: "debug", Description: "Reproduce a bug from a report, then find its root cause"},
				{Name: "design-review", Description: "Review a UI change for spacing, hierarchy and contrast"},
				{Name: "docs", Description: "Write or update documentation for a change"},
			}
		case ui.MenuMentions:
			return []ui.SlashItem{
				{Name: "internal/billing/consumer.go"},
				{Name: "internal/billing/webhook.go"},
				{Name: "internal/billing/queue_test.go"},
			}
		}
		return nil
	}
	ui.SendPromptHook = func(int64, string, []ui.Attachment, string, string) error { return nil }

	keys := ""
	dumpView := false
	for i, arg := range os.Args {
		if arg == "--keys" && i+1 < len(os.Args) {
			keys = os.Args[i+1]
		}
		if arg == "--view" {
			dumpView = true
		}
	}

	// A non-running executor, as the live path builds: the detail view asks it
	// for pane and PR state on open, and a nil one panics there.
	exec := executor.New(database, config.New(database))

	if keys != "" || dumpView {
		runDebugState(database, exec, keys, dumpView)
		return
	}

	model := ui.NewAppModel(database, exec, ".", "0.0.1")
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fail(err)
	}
}

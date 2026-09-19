package bbstore

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bborn/bb-tui/internal/bbapi"
	"github.com/bborn/bb-tui/internal/db"
)

const detailLogLimit = 400

func (s *Store) snapshot() []*db.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*db.Task, len(s.tasks))
	copy(out, s.tasks)
	return out
}

func matchesStatus(task *db.Task, opts db.ListTasksOptions) bool {
	if opts.Status == "" {
		if task.Status == db.StatusArchived {
			return false
		}
		// The board asks for open work and finished work separately and joins
		// the two lists, so an unfiltered call must not also return done tasks
		// or every finished thread appears on the board twice.
		if !opts.IncludeClosed && task.Status == db.StatusDone {
			return false
		}
		return true
	}
	if opts.Status == db.StatusQueued {
		return db.IsInProgress(task.Status)
	}
	return task.Status == opts.Status
}

func (s *Store) ListTasks(opts db.ListTasksOptions) ([]*db.Task, error) {
	tasks := s.snapshot()
	out := make([]*db.Task, 0, len(tasks))
	for _, task := range tasks {
		if !matchesStatus(task, opts) {
			continue
		}
		if opts.Project != "" && task.Project != opts.Project {
			continue
		}
		out = append(out, task)
	}

	if !opts.OrderByRecency {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Pinned != out[j].Pinned {
				return out[i].Pinned
			}
			return out[i].UpdatedAt.Time.After(out[j].UpdatedAt.Time)
		})
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

func (s *Store) GetTask(id int64) (*db.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("task #%d not found", id)
	}
	clone := *task
	return &clone, nil
}

func (s *Store) SearchTasks(query string, limit int) ([]*db.Task, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	tasks := s.snapshot()
	out := make([]*db.Task, 0, len(tasks))
	for _, task := range tasks {
		if needle == "" ||
			strings.Contains(strings.ToLower(task.Title), needle) ||
			strings.Contains(strings.ToLower(task.Project), needle) {
			out = append(out, task)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) CountTasksByStatus(status string) (int, error) {
	count := 0
	for _, task := range s.snapshot() {
		if status == db.StatusQueued {
			if db.IsInProgress(task.Status) {
				count++
			}
			continue
		}
		if task.Status == status {
			count++
		}
	}
	return count, nil
}

func (s *Store) CountTasksByProject(projectName string) (int, error) {
	count := 0
	for _, task := range s.snapshot() {
		if task.Project == projectName && task.Status != db.StatusArchived {
			count++
		}
	}
	return count, nil
}

func (s *Store) ListProjects() ([]*db.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*db.Project, len(s.projects))
	copy(out, s.projects)
	return out, nil
}

// MarkOpened records which thread the user is reading. Only that thread's
// conversation is fetched, because a timeline per thread per tick would be a
// hundred requests a second for no one's benefit.
func (s *Store) MarkOpened(taskID int64) error {
	s.logsMu.Lock()
	changed := s.openTask != taskID
	s.openTask = taskID
	s.logsMu.Unlock()
	if changed {
		go s.refreshLogs(taskID)
		go s.Context(taskID)
	}
	return nil
}

func (s *Store) OpenTask() int64 {
	s.logsMu.RLock()
	defer s.logsMu.RUnlock()
	return s.openTask
}

func (s *Store) GetTaskLogs(taskID int64, limit int) ([]*db.TaskLog, error) {
	s.logsMu.RLock()
	logs := s.logs[taskID]
	s.logsMu.RUnlock()

	// Never fetch on this path: it runs on the UI goroutine, and a round trip
	// here freezes the whole program until bb answers. Ask in the background and
	// let the next tick pick the answer up.
	if logs == nil {
		s.requestLogs(taskID)
	}

	if limit > 0 && len(logs) > limit {
		logs = logs[len(logs)-limit:]
	}
	out := make([]*db.TaskLog, len(logs))
	copy(out, logs)
	return out, nil
}

func (s *Store) GetTaskLogCount(taskID int64) (int, error) {
	s.logsMu.RLock()
	defer s.logsMu.RUnlock()
	return len(s.logs[taskID]), nil
}

func (s *Store) GetLatestLogPerTask(taskIDs []int64) (map[int64]*db.TaskLog, error) {
	out := map[int64]*db.TaskLog{}
	s.logsMu.RLock()
	defer s.logsMu.RUnlock()
	for _, id := range taskIDs {
		logs := s.logs[id]
		if len(logs) > 0 {
			out[id] = logs[len(logs)-1]
		}
	}
	return out, nil
}

// RefreshOpenLogs re-reads the conversation of whichever thread is on screen.
func (s *Store) RefreshOpenLogs() {
	if taskID := s.OpenTask(); taskID != 0 {
		_ = s.refreshLogs(taskID)
	}
}

func (s *Store) requestLogs(taskID int64) {
	s.logsMu.Lock()
	if s.fetching == nil {
		s.fetching = map[int64]bool{}
	}
	if s.fetching[taskID] {
		s.logsMu.Unlock()
		return
	}
	s.fetching[taskID] = true
	s.logsMu.Unlock()

	go func() {
		_ = s.refreshLogs(taskID)
		s.logsMu.Lock()
		delete(s.fetching, taskID)
		s.logsMu.Unlock()
	}()
}

func (s *Store) refreshLogs(taskID int64) error {
	threadID, err := s.ThreadID(taskID)
	if err != nil {
		return err
	}
	timeline, err := s.client.Timeline(threadID, 60)
	if err != nil {
		return err
	}

	logs := make([]*db.TaskLog, 0, len(timeline.Rows))
	for index, row := range timeline.Rows {
		lineType, content, ok := LogLineFor(row)
		if !ok {
			continue
		}
		created := row.CreatedAt
		if created == 0 {
			created = row.StartedAt
		}
		logs = append(logs, &db.TaskLog{
			ID:        int64(index + 1),
			TaskID:    taskID,
			LineType:  lineType,
			Content:   content,
			CreatedAt: db.LocalTime{Time: time.UnixMilli(created)},
		})
	}

	s.logsMu.Lock()
	s.logs[taskID] = logs
	s.logsMu.Unlock()
	return nil
}

// Activity reports whether a thread is working and what it is doing, for the
// indicator the thread view shows while a turn is in flight.
func (s *Store) Activity(taskID int64) (bool, string) {
	s.mu.RLock()
	task, ok := s.byID[taskID]
	s.mu.RUnlock()
	if !ok {
		return false, ""
	}

	working := task.Status == db.StatusProcessing
	if !working {
		return false, ""
	}
	return true, compactSince(task.UpdatedAt.Time)
}

// ComposerContext is what the app shows around its composer: where the thread
// runs, which model answers, and how much it is allowed to do.
type ComposerContext struct {
	Project     string
	Environment string
	Provider    string
	Model       string
	Reasoning   string
	Permission  string
}

// Context reads the execution options bb would apply to the next message.
func (s *Store) Context(taskID int64) ComposerContext {
	s.mu.RLock()
	task, ok := s.byID[taskID]
	s.mu.RUnlock()
	if !ok {
		return ComposerContext{}
	}

	context := ComposerContext{
		Project:     task.Project,
		Environment: task.BranchName,
		Provider:    task.Executor,
	}

	threadID, err := s.ThreadID(taskID)
	if err != nil {
		return context
	}

	s.optionsMu.RLock()
	cached, seen := s.options[taskID]
	s.optionsMu.RUnlock()
	if seen {
		context.Model = s.modelDisplayName(task.Executor, cached.Model)
		context.Reasoning = cached.ReasoningLevel
		context.Permission = cached.PermissionMode
		return context
	}

	go func() {
		options, optErr := s.client.ExecutionOptions(threadID)
		if optErr != nil {
			return
		}
		s.optionsMu.Lock()
		if s.options == nil {
			s.options = map[int64]bbapi.ExecutionOptions{}
		}
		s.options[taskID] = options
		s.optionsMu.Unlock()
	}()

	return context
}

// Skills lists the project's skills, cached because the catalogue is large and
// does not change while a message is being typed.
func (s *Store) Skills(taskID int64) []bbapi.Skill {
	s.mu.RLock()
	task, ok := s.byID[taskID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}

	s.skillsMu.RLock()
	cached, seen := s.skills[task.Project]
	s.skillsMu.RUnlock()
	if seen {
		return cached
	}

	projectID, err := s.ProjectIDForName(task.Project)
	if err != nil {
		return nil
	}
	skills, err := s.client.Skills(projectID, "")
	if err != nil {
		return nil
	}

	s.skillsMu.Lock()
	if s.skills == nil {
		s.skills = map[string][]bbapi.Skill{}
	}
	s.skills[task.Project] = skills
	s.skillsMu.Unlock()
	return skills
}

// SearchFiles asks bb to match paths, so a large repository costs one request
// rather than a full tree in memory.
func (s *Store) SearchFiles(taskID int64, query string, limit int) []bbapi.ProjectFile {
	s.mu.RLock()
	task, ok := s.byID[taskID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	projectID, err := s.ProjectIDForName(task.Project)
	if err != nil {
		return nil
	}
	files, err := s.client.Files(projectID, query, limit)
	if err != nil {
		return nil
	}
	return files
}

// compactSince is how long a turn has been running, in the same shorthand the
// board uses for ages.
func compactSince(started time.Time) string {
	elapsed := time.Since(started)
	switch {
	case elapsed < time.Minute:
		return strconv.Itoa(int(elapsed.Seconds())) + "s"
	case elapsed < time.Hour:
		return strconv.Itoa(int(elapsed.Minutes())) + "m"
	default:
		return strconv.Itoa(int(elapsed.Hours())) + "h"
	}
}

// modelDisplayName prefers what bb calls a model over anything derived from its
// id: "claude-fable-5-1" is "Fable 5.1", which no amount of splitting on dashes
// will produce.
func (s *Store) modelDisplayName(providerID, modelID string) string {
	if modelID == "" {
		return ""
	}

	s.modelsMu.RLock()
	names, seen := s.modelNames[providerID]
	s.modelsMu.RUnlock()

	if !seen {
		options, err := s.client.Options(providerID)
		if err != nil {
			return modelID
		}
		names = map[string]string{}
		for _, model := range options.Models {
			names[model.ID] = model.DisplayName
		}
		s.modelsMu.Lock()
		if s.modelNames == nil {
			s.modelNames = map[string]map[string]string{}
		}
		s.modelNames[providerID] = names
		s.modelsMu.Unlock()
	}

	if name, ok := names[modelID]; ok && name != "" {
		return name
	}
	return modelID
}

package bbstore

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bborn/bb-tui/internal/bbapi"
	"github.com/bborn/bb-tui/internal/db"
)

// Store serves the board from bb. It holds threads in memory rather than in a
// local table so there is exactly one source of truth: nothing here is written
// back, and anything stale is replaced by the next refresh.
//
// The cache exists because the UI asks for the task list on every tick, and a
// round trip per frame would be visible.
type Store struct {
	client *bbapi.Client

	mu        sync.RWMutex
	tasks     []*db.Task
	byID      map[int64]*db.Task
	threadIDs map[int64]string
	taskIDs   map[string]int64
	projects  []*db.Project
	nextID    int64

	modelsMu   sync.RWMutex
	modelNames map[string]map[string]string

	skillsMu sync.RWMutex
	skills   map[string][]bbapi.Skill

	optionsMu sync.RWMutex
	options   map[int64]bbapi.ExecutionOptions

	logsMu   sync.RWMutex
	logs     map[int64][]*db.TaskLog
	fetching map[int64]bool
	openTask int64

	lastErr error
}

func New(client *bbapi.Client) *Store {
	return &Store{
		client:    client,
		byID:      map[int64]*db.Task{},
		threadIDs: map[int64]string{},
		taskIDs:   map[string]int64{},
		logs:      map[int64][]*db.TaskLog{},
	}
}

// ThreadID maps a board row back to the thread it came from.
func (s *Store) ThreadID(taskID int64) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	threadID, ok := s.threadIDs[taskID]
	if !ok {
		return "", fmt.Errorf("task #%d is not a bb thread", taskID)
	}
	return threadID, nil
}

// TaskID assigns a stable board number to a thread, so a row keeps its number
// for the life of the session.
func (s *Store) TaskID(threadID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.taskIDLocked(threadID)
}

func (s *Store) taskIDLocked(threadID string) int64 {
	if id, ok := s.taskIDs[threadID]; ok {
		return id
	}
	s.nextID++
	s.taskIDs[threadID] = s.nextID
	s.threadIDs[s.nextID] = threadID
	return s.nextID
}

func statusFor(thread bbapi.Thread) string {
	if thread.ArchivedAt != nil {
		return db.StatusArchived
	}
	if thread.HasPendingInteraction ||
		thread.QueuedWork == "failed" ||
		thread.Runtime.DisplayStatus == "error" ||
		thread.Runtime.DisplayStatus == "host-reconnecting" {
		return db.StatusBlocked
	}
	switch thread.Runtime.DisplayStatus {
	case "starting", "active", "stopping":
		return db.StatusProcessing
	case "pending", "provisioning", "waiting-for-host":
		return db.StatusBacklog
	}
	if thread.QueuedWork == "waiting" {
		return db.StatusQueued
	}
	return db.StatusDone
}

func titleFor(thread bbapi.Thread) string {
	if thread.Title != nil && strings.TrimSpace(*thread.Title) != "" {
		return *thread.Title
	}
	if thread.TitleFallback != nil && strings.TrimSpace(*thread.TitleFallback) != "" {
		return *thread.TitleFallback
	}
	return "Untitled"
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Refresh pulls threads and projects and rebuilds the cache.
func (s *Store) Refresh() error {
	projects, err := s.client.ListProjects()
	if err != nil {
		s.setErr(err)
		return err
	}
	threads, err := s.client.ListThreads()
	if err != nil {
		s.setErr(err)
		return err
	}

	names := make(map[string]string, len(projects))
	projectRows := make([]*db.Project, 0, len(projects))
	for index, project := range projects {
		names[project.ID] = project.Name
		projectRows = append(projectRows, &db.Project{
			ID:   int64(index + 1),
			Name: project.Name,
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*db.Task, 0, len(threads))
	byID := make(map[int64]*db.Task, len(threads))
	for _, thread := range threads {
		id := s.taskIDLocked(thread.ID)
		task := &db.Task{
			ID:             id,
			Title:          titleFor(thread),
			Status:         statusFor(thread),
			Type:           "code",
			Project:        names[thread.ProjectID],
			Executor:       thread.ProviderID,
			BranchName:     deref(thread.EnvironmentBranchName),
			WorktreePath:   deref(thread.EnvironmentPath),
			Pinned:         thread.PinnedAt != nil,
			PermissionMode: "",
			CreatedAt:      db.LocalTime{Time: time.UnixMilli(thread.CreatedAt)},
			UpdatedAt:      db.LocalTime{Time: time.UnixMilli(thread.UpdatedAt)},
		}
		tasks = append(tasks, task)
		byID[id] = task
	}

	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.Time.After(tasks[j].UpdatedAt.Time)
	})

	s.tasks = tasks
	s.byID = byID
	s.projects = projectRows
	s.lastErr = nil
	return nil
}

func (s *Store) setErr(err error) {
	s.mu.Lock()
	s.lastErr = err
	s.mu.Unlock()
}

// Err reports the most recent refresh failure, if the board is showing stale
// data because bb went away.
func (s *Store) Err() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastErr
}

// ProjectIDForName resolves the project name a board row carries back to bb's
// project id.
func (s *Store) ProjectIDForName(name string) (string, error) {
	projects, err := s.client.ListProjects()
	if err != nil {
		return "", err
	}
	for _, project := range projects {
		if project.Name == name {
			return project.ID, nil
		}
	}
	return "", fmt.Errorf("no bb project named %q", name)
}

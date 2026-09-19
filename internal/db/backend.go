package db

// Backend supplies tasks, their logs, and projects from somewhere other than
// this process's SQLite file. bb-tui installs one so the board reads bb's
// threads directly: with a backend set there is no local copy of a task to fall
// out of date, and the SQLite file holds only state bb has no opinion about —
// themes, saved views, keybindings, settings.
type Backend interface {
	ListTasks(opts ListTasksOptions) ([]*Task, error)
	GetTask(id int64) (*Task, error)
	SearchTasks(query string, limit int) ([]*Task, error)
	CountTasksByStatus(status string) (int, error)
	CountTasksByProject(projectName string) (int, error)
	GetTaskLogs(taskID int64, limit int) ([]*TaskLog, error)
	GetLatestLogPerTask(taskIDs []int64) (map[int64]*TaskLog, error)
	GetTaskLogCount(taskID int64) (int, error)
	ListProjects() ([]*Project, error)
	MarkOpened(taskID int64) error
}

// SetBackend routes task and project reads through b. Passing nil restores the
// local tables.
func (db *DB) SetBackend(b Backend) { db.backend = b }

// HasBackend reports whether reads are served from somewhere other than SQLite.
func (db *DB) HasBackend() bool { return db.backend != nil }

// PurgeBackedTables empties the local copies of anything the backend now owns.
// Without this a previous run's rows sit in the file unread, which is the exact
// duplicated state a backend exists to remove.
func (db *DB) PurgeBackedTables() error {
	for _, statement := range []string{
		`DELETE FROM task_logs`,
		`DELETE FROM task_attachments`,
		`DELETE FROM tasks`,
		`DELETE FROM projects`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

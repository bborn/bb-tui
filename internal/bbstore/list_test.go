package bbstore

import (
	"testing"
	"time"

	"github.com/bborn/bb-tui/internal/bbapi"
	"github.com/bborn/bb-tui/internal/db"
)

func storeWith(tasks ...*db.Task) *Store {
	store := New(bbapi.New("http://127.0.0.1:1"))
	store.tasks = tasks
	store.byID = map[int64]*db.Task{}
	for _, task := range tasks {
		store.byID[task.ID] = task
	}
	return store
}

func task(id int64, status string, mutate func(*db.Task)) *db.Task {
	t := &db.Task{
		ID:        id,
		Status:    status,
		Project:   "acme",
		UpdatedAt: db.LocalTime{Time: time.Unix(id, 0)},
	}
	if mutate != nil {
		mutate(t)
	}
	return t
}

// The board asks for open work and finished work separately and joins the two
// lists, so an unfiltered call must not also return done tasks.
func TestListTasksExcludesDoneUnlessAsked(t *testing.T) {
	store := storeWith(
		task(1, db.StatusProcessing, nil),
		task(2, db.StatusDone, nil),
		task(3, db.StatusArchived, nil),
	)

	open, _ := store.ListTasks(db.ListTasksOptions{Limit: -1})
	if len(open) != 1 || open[0].ID != 1 {
		t.Fatalf("an unfiltered list should be open work only, got %+v", open)
	}

	withClosed, _ := store.ListTasks(db.ListTasksOptions{Limit: -1, IncludeClosed: true})
	if len(withClosed) != 2 {
		t.Fatalf("IncludeClosed should add done but never archived, got %d", len(withClosed))
	}
}

func TestListTasksStatusQueuedMeansInProgress(t *testing.T) {
	store := storeWith(
		task(1, db.StatusQueued, nil),
		task(2, db.StatusProcessing, nil),
		task(3, db.StatusDone, nil),
	)

	got, _ := store.ListTasks(db.ListTasksOptions{Status: db.StatusQueued, Limit: -1})
	if len(got) != 2 {
		t.Fatalf("the In Progress column is queued plus processing, got %d", len(got))
	}
}

func TestListTasksSortsPinnedFirstThenRecent(t *testing.T) {
	store := storeWith(
		task(1, db.StatusProcessing, nil),
		task(2, db.StatusProcessing, func(t *db.Task) { t.Pinned = true }),
		task(3, db.StatusProcessing, nil),
	)

	got, _ := store.ListTasks(db.ListTasksOptions{Limit: -1})
	if got[0].ID != 2 {
		t.Fatalf("a pinned task should lead, got %d", got[0].ID)
	}
	if got[1].ID != 3 {
		t.Fatalf("the rest should be most-recent first, got %d", got[1].ID)
	}
}

func TestListTasksRecencyOrderIgnoresPinning(t *testing.T) {
	store := storeWith(
		task(1, db.StatusProcessing, nil),
		task(2, db.StatusProcessing, func(t *db.Task) { t.Pinned = true }),
	)

	got, _ := store.ListTasks(db.ListTasksOptions{Limit: -1, OrderByRecency: true})
	if got[0].ID != 1 {
		t.Fatalf("OrderByRecency should keep the store's order, got %d", got[0].ID)
	}
}

func TestListTasksFiltersByProjectAndLimit(t *testing.T) {
	store := storeWith(
		task(1, db.StatusProcessing, nil),
		task(2, db.StatusProcessing, func(t *db.Task) { t.Project = "other" }),
		task(3, db.StatusProcessing, nil),
	)

	got, _ := store.ListTasks(db.ListTasksOptions{Project: "acme", Limit: -1})
	if len(got) != 2 {
		t.Fatalf("project filter wrong, got %d", len(got))
	}

	limited, _ := store.ListTasks(db.ListTasksOptions{Limit: 1})
	if len(limited) != 1 {
		t.Fatalf("limit ignored, got %d", len(limited))
	}
}

func TestGetTaskReportsAMissingThread(t *testing.T) {
	store := storeWith(task(1, db.StatusProcessing, nil))
	if _, err := store.GetTask(99); err == nil {
		t.Fatal("a task the store does not hold should be an error, not a zero value")
	}
}

func TestThreadIDRoundTripsAndIsStable(t *testing.T) {
	store := New(bbapi.New("http://127.0.0.1:1"))

	first := store.TaskID("thr_abc")
	again := store.TaskID("thr_abc")
	other := store.TaskID("thr_def")

	if first != again {
		t.Fatalf("a thread should keep its board number, got %d then %d", first, again)
	}
	if first == other {
		t.Fatal("two threads should not share a board number")
	}

	id, err := store.ThreadID(first)
	if err != nil || id != "thr_abc" {
		t.Fatalf("board number should map back to its thread, got %q %v", id, err)
	}
	if _, err := store.ThreadID(9999); err == nil {
		t.Fatal("an unknown board number should error")
	}
}

func TestCountTasksByStatus(t *testing.T) {
	store := storeWith(
		task(1, db.StatusProcessing, nil),
		task(2, db.StatusQueued, nil),
		task(3, db.StatusBlocked, nil),
	)

	if got, _ := store.CountTasksByStatus(db.StatusQueued); got != 2 {
		t.Fatalf("In Progress count should include processing, got %d", got)
	}
	if got, _ := store.CountTasksByStatus(db.StatusBlocked); got != 1 {
		t.Fatalf("blocked count wrong, got %d", got)
	}
}

package bbstore

import (
	"testing"

	"github.com/bborn/bb-tui/internal/bbapi"
	"github.com/bborn/bb-tui/internal/db"
)

func thread(display string, mutate func(*bbapi.Thread)) bbapi.Thread {
	t := bbapi.Thread{Runtime: bbapi.Runtime{DisplayStatus: display}, QueuedWork: "none"}
	if mutate != nil {
		mutate(&t)
	}
	return t
}

func TestStatusForMapsEveryRuntimeStatus(t *testing.T) {
	cases := []struct {
		display string
		want    string
	}{
		{"pending", db.StatusBacklog},
		{"provisioning", db.StatusBacklog},
		{"waiting-for-host", db.StatusBacklog},
		{"starting", db.StatusProcessing},
		{"active", db.StatusProcessing},
		{"stopping", db.StatusProcessing},
		{"error", db.StatusBlocked},
		{"host-reconnecting", db.StatusBlocked},
		{"idle", db.StatusDone},
	}

	for _, test := range cases {
		if got := statusFor(thread(test.display, nil)); got != test.want {
			t.Errorf("statusFor(%q) = %q, want %q", test.display, got, test.want)
		}
	}
}

func TestStatusForPutsAnInteractionAheadOfRunning(t *testing.T) {
	// A thread waiting on an answer is not making progress, so it belongs in
	// the column the reader is meant to act on.
	got := statusFor(thread("active", func(t *bbapi.Thread) {
		t.HasPendingInteraction = true
	}))
	if got != db.StatusBlocked {
		t.Fatalf("an active thread awaiting an answer should be blocked, got %q", got)
	}
}

func TestStatusForTreatsAFailedQueueAsBlocking(t *testing.T) {
	got := statusFor(thread("idle", func(t *bbapi.Thread) { t.QueuedWork = "failed" }))
	if got != db.StatusBlocked {
		t.Fatalf("a failed queued message should block, got %q", got)
	}
}

func TestStatusForTreatsWaitingQueueAsQueued(t *testing.T) {
	got := statusFor(thread("idle", func(t *bbapi.Thread) { t.QueuedWork = "waiting" }))
	if got != db.StatusQueued {
		t.Fatalf("a waiting queued message should queue, got %q", got)
	}
}

func TestStatusForArchivedWinsOverEverything(t *testing.T) {
	archivedAt := int64(1)
	got := statusFor(thread("active", func(t *bbapi.Thread) {
		t.ArchivedAt = &archivedAt
		t.HasPendingInteraction = true
	}))
	if got != db.StatusArchived {
		t.Fatalf("an archived thread should read as archived, got %q", got)
	}
}

func TestTitleForFallsBackBeforeGivingUp(t *testing.T) {
	title := "real title"
	fallback := "fallback"
	blank := "   "

	if got := titleFor(bbapi.Thread{Title: &title}); got != title {
		t.Errorf("a title should be used, got %q", got)
	}
	if got := titleFor(bbapi.Thread{Title: &blank, TitleFallback: &fallback}); got != fallback {
		t.Errorf("a blank title should fall back, got %q", got)
	}
	if got := titleFor(bbapi.Thread{}); got != "Untitled" {
		t.Errorf("no title at all should read Untitled, got %q", got)
	}
}

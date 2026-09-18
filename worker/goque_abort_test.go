package worker_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/qor5/admin/v3/worker"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func abortTestQueue(t *testing.T, job *stubJob) worker.Queue {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	q := worker.NewGoQueQueue(db)
	if err := q.Listen([]*worker.QorJobDefinition{{
		Name: job.name, Handler: job.handler, Concurrency: 1,
	}}, func(uint) (worker.QueJobInterface, error) {
		return job, nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Shutdown(context.Background()) })
	return q
}

func awaitStatus(t *testing.T, job *stubJob, want string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if job.GetStatus() == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("status = %q, want %q", job.GetStatus(), want)
}

// TestGoQueCancelStopsARunningJob: aborting a job whose row reads new or
// scheduled takes the Remove path and writes "cancelled". A running job can
// read that — "Run now" relabels a running instance "scheduled" — and the
// watcher used to stop only on "killed", so the handler kept its worker's
// single slot while the listing called it cancelled.
func TestGoQueCancelStopsARunningJob(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	job := &stubJob{
		name: "abortCancelTestJob",
		id:   "11",
		handler: func(ctx context.Context, _ worker.QorJobInterface) error {
			close(started)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		},
	}
	job.SetStatus(worker.JobStatusNew)
	q := abortTestQueue(t, job)
	if err := q.Add(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(45 * time.Second):
		t.Fatal("job never started")
	}
	if err := q.Remove(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("handler still running after the job was cancelled")
	}
	// The handler returns ctx.Err(); that must not turn the cancel into an
	// "exception".
	time.Sleep(500 * time.Millisecond)
	if got := job.GetStatus(); got != worker.JobStatusCancelled {
		t.Fatalf("status after cancel = %q, want %q", got, worker.JobStatusCancelled)
	}
}

// TestGoQueStaleEntryKeepsFinishedStatus: a second queue entry for a run that
// already finished must not relabel it. It used to mark a done job "killed".
func TestGoQueStaleEntryKeepsFinishedStatus(t *testing.T) {
	ran := make(chan struct{}, 4)
	job := &stubJob{
		name: "staleEntryTestJob",
		id:   "12",
		handler: func(context.Context, worker.QorJobInterface) error {
			ran <- struct{}{}
			return nil
		},
	}
	job.SetStatus(worker.JobStatusNew)
	q := abortTestQueue(t, job)
	if err := q.Add(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, job, worker.JobStatusDone)

	// Enqueue a duplicate without resetting the status, as a second click did.
	if err := q.Add(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	if got := job.GetStatus(); got != worker.JobStatusDone {
		t.Fatalf("status after a stale entry = %q, want %q", got, worker.JobStatusDone)
	}
	if n := len(ran); n != 1 {
		t.Fatalf("handler ran %d times, want 1", n)
	}
}

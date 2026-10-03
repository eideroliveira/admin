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

// TestGoQueReportsAnInterruptedInstance: after a restart the queue redelivers
// the entry whose performer died, and the instance it points at still reads
// "running". That instance is marked killed — as before — and handed to
// OnInterrupted, instead of disappearing with nothing said; its handler never
// runs. An instance a person stopped already reads "killed" and is not
// reported.
func TestGoQueReportsAnInterruptedInstance(t *testing.T) {
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

	ran := make(chan struct{}, 4)
	interrupted := make(chan string, 4)
	q := worker.NewGoQueQueue(db)
	job := &stubJob{
		name: "interruptedTestJob",
		id:   "7",
		handler: func(ctx context.Context, j worker.QorJobInterface) error {
			ran <- struct{}{}
			return nil
		},
	}
	// The status the next delivery finds, as the database would have it.
	delivered := make(chan string, 4)
	jobDefs := []*worker.QorJobDefinition{{
		Name:        job.name,
		Handler:     job.handler,
		Concurrency: 1,
		OnInterrupted: func(ctx context.Context, j worker.QueJobInterface) {
			interrupted <- j.GetStatus()
		},
	}}
	if err := q.Listen(jobDefs, func(uint) (worker.QueJobInterface, error) {
		job.SetStatus(<-delivered)
		return job, nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Shutdown(context.Background()) })

	deliver := func(status string) {
		t.Helper()
		delivered <- status
		if err := q.Add(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}

	// Left "running" by a process that died.
	deliver(worker.JobStatusRunning)
	select {
	case got := <-interrupted:
		if got != worker.JobStatusKilled {
			t.Errorf("the callback must see the instance already marked killed, got %q", got)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("an instance left running was not reported as interrupted")
	}
	select {
	case <-ran:
		t.Error("an interrupted instance must not be performed again by the stale entry")
	case <-time.After(2 * time.Second):
	}

	// Stopped by a person: already "killed" on delivery — not an interruption.
	deliver(worker.JobStatusKilled)
	select {
	case <-interrupted:
		t.Error("an instance someone stopped must not be reported as interrupted")
	case <-time.After(5 * time.Second):
	}
}

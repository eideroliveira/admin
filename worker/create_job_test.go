package worker_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/qor5/admin/v3/worker"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// failingQueue refuses every enqueue, standing in for a queue whose own
// connection is down — the state that used to leave a job instance committed
// with nothing to run it.
type failingQueue struct {
	worker.Queue
	err error
}

func (q *failingQueue) Add(context.Context, worker.QueJobInterface) error { return q.err }
func (q *failingQueue) Kill(context.Context, worker.QueJobInterface) error {
	return nil
}
func (q *failingQueue) Remove(context.Context, worker.QueJobInterface) error { return nil }
func (q *failingQueue) Listen([]*worker.QorJobDefinition, func(uint) (worker.QueJobInterface, error)) error {
	return nil
}
func (q *failingQueue) Shutdown(context.Context) error { return nil }

type testArg struct {
	Name string
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := worker.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Exec("DELETE FROM qor_job_instances")
	db.Exec("DELETE FROM qor_jobs")
	return db
}

// TestCreateSystemJobFailsTheInstanceWhenEnqueueFails is the regression for
// jobs stuck at "queued" forever. The instance used to be written on the
// builder's own connection while the job row rode the creating transaction, so
// a failed enqueue rolled back the job row and left the instance behind at
// "new": every listing reported it as queued, and nothing would ever run it.
func TestCreateSystemJobFailsTheInstanceWhenEnqueueFails(t *testing.T) {
	db := openTestDB(t)
	wantErr := errors.New("queue is down")
	b := worker.NewWithQueue(db, &failingQueue{err: wantErr})
	b.NewJob("Test Job").Resource(&testArg{})

	_, err := b.CreateSystemJob(context.Background(), "Test Job", &testArg{Name: "x"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("CreateSystemJob err = %v, want %v", err, wantErr)
	}

	var queued int64
	if err := db.Model(&worker.QorJobInstance{}).
		Where("status IN ?", []string{worker.JobStatusNew, worker.JobStatusScheduled}).
		Count(&queued).Error; err != nil {
		t.Fatalf("count instances: %v", err)
	}
	if queued != 0 {
		t.Fatalf("%d instance(s) left reading as queued after a failed enqueue", queued)
	}

	// Nor may an instance outlive the job row the worker resolves it through.
	var dangling int64
	if err := db.Model(&worker.QorJobInstance{}).
		Joins("LEFT JOIN qor_jobs ON qor_jobs.id = qor_job_instances.qor_job_id").
		Where("qor_jobs.id IS NULL").Count(&dangling).Error; err != nil {
		t.Fatalf("count dangling: %v", err)
	}
	if dangling != 0 {
		t.Fatalf("%d instance(s) left without a qor_jobs row", dangling)
	}
}

// TestCreateSystemJobEnqueuesAfterCommit pins the ordering the queue depends
// on: by the time an entry exists, the rows a worker resolves it through are
// committed and visible on another connection. Enqueueing inside the
// transaction let a worker lock the entry first, find nothing, and expire it —
// permanently, since plans carry no retry policy.
func TestCreateSystemJobEnqueuesAfterCommit(t *testing.T) {
	db := openTestDB(t)

	var seen struct {
		jobs, instances int64
	}
	q := &observingQueue{onAdd: func(inst worker.QueJobInterface) {
		// A separate connection: it can only see committed rows.
		other, err := gorm.Open(postgres.Open(os.Getenv("TEST_DATABASE_URL")), &gorm.Config{})
		if err != nil {
			t.Errorf("open second connection: %v", err)
			return
		}
		other.Model(&worker.QorJob{}).Count(&seen.jobs)
		other.Model(&worker.QorJobInstance{}).Count(&seen.instances)
	}}
	b := worker.NewWithQueue(db, q)
	b.NewJob("Test Job").Resource(&testArg{})

	if _, err := b.CreateSystemJob(context.Background(), "Test Job", &testArg{Name: "x"}); err != nil {
		t.Fatalf("CreateSystemJob: %v", err)
	}
	if seen.jobs != 1 || seen.instances != 1 {
		t.Fatalf("at enqueue time another connection saw %d job(s) and %d instance(s), want 1 and 1",
			seen.jobs, seen.instances)
	}
}

type observingQueue struct {
	failingQueue
	onAdd func(worker.QueJobInterface)
}

func (q *observingQueue) Add(_ context.Context, inst worker.QueJobInterface) error {
	q.onAdd(inst)
	return nil
}

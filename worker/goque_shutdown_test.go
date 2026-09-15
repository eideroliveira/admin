package worker_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/qor5/admin/v3/worker"
	"go.uber.org/multierr"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestGoQueShutdownStopsWorkersConcurrently pins why Shutdown stops its workers
// in parallel. que.Worker.Stop waits for the jobs that worker is performing and
// never cancels them, so when the workers were stopped one after another, a
// single job outliving the deadline made every worker queued after it fail at
// once with ctx.Err() — its own jobs long finished — and the shutdown reported
// one error per queue. Only the worker whose job is still running may fail.
func TestGoQueShutdownStopsWorkersConcurrently(t *testing.T) {
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

	const quickWorkers = 9
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	slowStarted := make(chan struct{}, 1)
	quickDone := make(chan string, quickWorkers)

	jobs := map[uint]*stubJob{}
	var defs []*worker.QorJobDefinition
	for i := 0; i <= quickWorkers; i++ {
		id := uint(i + 1)
		job := &stubJob{name: fmt.Sprintf("shutdownConcurrent%d_%s", i, suffix), id: strconv.Itoa(int(id))}
		if i == 0 {
			// The slow one: still performing when the deadline passes.
			job.handler = func(ctx context.Context, _ worker.QorJobInterface) error {
				slowStarted <- struct{}{}
				<-release
				return nil
			}
		} else {
			job.handler = func(ctx context.Context, j worker.QorJobInterface) error {
				info, _ := j.GetJobInfo()
				quickDone <- info.JobID
				return nil
			}
		}
		jobs[id] = job
		defs = append(defs, &worker.QorJobDefinition{Name: job.name, Handler: job.handler, Concurrency: 1})
	}

	q := worker.NewGoQueQueue(db)
	if err := q.Listen(defs, func(id uint) (worker.QueJobInterface, error) {
		job := jobs[id]
		job.SetStatus(worker.JobStatusNew)
		return job, nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, job := range jobs {
		job.SetStatus(worker.JobStatusNew)
		if err := q.Add(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	waitFor := func(what string, ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(45 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}
	waitFor("the slow job to start", slowStarted)
	for i := 0; i < quickWorkers; i++ {
		select {
		case <-quickDone:
		case <-time.After(45 * time.Second):
			t.Fatalf("only %d of %d quick jobs ran", i, quickWorkers)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	began := time.Now()
	err = q.Shutdown(ctx)
	took := time.Since(began)

	errs := multierr.Errors(err)
	if len(errs) != 1 {
		t.Fatalf("Shutdown returned %d errors (%v), want exactly 1: only the worker still performing may miss the deadline", len(errs), err)
	}
	if took > 4*time.Second {
		t.Fatalf("Shutdown took %v with a 2s deadline", took)
	}
}

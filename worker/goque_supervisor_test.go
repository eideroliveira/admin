package worker_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/qor5/admin/v3/worker"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// stubJob is the minimum QueJobInterface the goque Perform path touches.
type stubJob struct {
	mu      sync.Mutex
	status  string
	name    string
	id      string
	handler worker.JobHandler
}

func (j *stubJob) GetJobInfo() (*worker.JobInfo, error) {
	return &worker.JobInfo{JobID: j.id, JobName: j.name, Argument: map[string]any{}}, nil
}
func (j *stubJob) SetProgress(uint) error               { return nil }
func (j *stubJob) SetProgressText(string) error         { return nil }
func (j *stubJob) AddLog(string) error                  { return nil }
func (j *stubJob) AddLogf(string, ...interface{}) error { return nil }
func (j *stubJob) StartRefresh()                        {}
func (j *stubJob) StopRefresh()                         {}
func (j *stubJob) GetHandler() worker.JobHandler        { return j.handler }
func (j *stubJob) FetchAndSetStatus() (string, error)   { return j.GetStatus(), nil }

func (j *stubJob) GetStatus() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

func (j *stubJob) SetStatus(s string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = s
	return nil
}

// TestGoQueWorkerRestartsAfterConnectionLoss reproduces the production failure:
// go-que's Run() gives up for good when its Postgres connection drops, which
// used to retire the queue for the lifetime of the process and leave every
// later job stuck at "new". The queue must serve jobs again afterwards.
func TestGoQueWorkerRestartsAfterConnectionLoss(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	// Tag the pool so the test only terminates its own backends.
	db, err := gorm.Open(postgres.Open(dsn+" application_name=worker_supervisor_test"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	ran := make(chan string, 8)
	q := worker.NewGoQueQueue(db)
	job := &stubJob{
		name: "supervisorTestJob",
		id:   "1",
		handler: func(ctx context.Context, j worker.QorJobInterface) error {
			info, _ := j.GetJobInfo()
			ran <- info.JobID
			return nil
		},
	}
	jobDefs := []*worker.QorJobDefinition{{
		Name:        job.name,
		Handler:     job.handler,
		Concurrency: 1,
	}}
	if err := q.Listen(jobDefs, func(uint) (worker.QueJobInterface, error) {
		// Each delivery gets a fresh "new" job; Perform rejects any other status.
		job.SetStatus(worker.JobStatusNew)
		return job, nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Shutdown(context.Background()) })

	enqueue := func() {
		job.SetStatus(worker.JobStatusNew)
		if err := q.Add(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	awaitRun := func(what string) {
		t.Helper()
		select {
		case <-ran:
		case <-time.After(45 * time.Second):
			t.Fatalf("job never ran %s", what)
		}
	}

	enqueue()
	awaitRun("before connection loss")

	// Kill every backend this pool owns, including the worker's, which is what
	// the broken pipe did in production.
	if err := db.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE application_name = 'worker_supervisor_test' AND pid <> pg_backend_pid()`).Error; err != nil {
		t.Logf("terminate returned %v (expected if it killed its own connection)", err)
	}

	enqueue()
	awaitRun("after connection loss — the worker was not restarted")
}

// TestGoQueShutdownStopsWorkers covers the other side of supervision: Shutdown
// backs the admin "pause system jobs" switch, so a supervisor must treat a
// stopped worker as intentional and not restart it.
func TestGoQueShutdownStopsWorkers(t *testing.T) {
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

	ran := make(chan string, 8)
	q := worker.NewGoQueQueue(db)
	job := &stubJob{
		name: "shutdownTestJob",
		id:   "2",
		handler: func(ctx context.Context, j worker.QorJobInterface) error {
			ran <- "ran"
			return nil
		},
	}
	if err := q.Listen([]*worker.QorJobDefinition{{
		Name: job.name, Handler: job.handler, Concurrency: 1,
	}}, func(uint) (worker.QueJobInterface, error) {
		job.SetStatus(worker.JobStatusNew)
		return job, nil
	}); err != nil {
		t.Fatal(err)
	}

	job.SetStatus(worker.JobStatusNew)
	if err := q.Add(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(45 * time.Second):
		t.Fatal("job never ran before shutdown")
	}

	if err := q.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	job.SetStatus(worker.JobStatusNew)
	if err := q.Add(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
		t.Fatal("job ran after shutdown — supervision defeats the pause switch")
	case <-time.After(5 * time.Second):
	}
}

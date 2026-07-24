package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/tnclong/go-que"
	"github.com/tnclong/go-que/pg"
	"go.uber.org/multierr"
	"gorm.io/gorm"
)

// A que.Worker's Run() returns for good on any error — most often a dropped
// Postgres connection — and go-que never restarts it. Left unsupervised, one
// broken pipe silently retires that queue for the lifetime of the process and
// every job enqueued afterwards sits at "new" forever. Each worker is therefore
// run under supervise(), which rebuilds and restarts it with exponential
// backoff until Shutdown.
const (
	workerRestartInitialBackoff = time.Second
	workerRestartMaxBackoff     = time.Minute
	// workerHealthyRunTime is how long a worker must stay up before its next
	// failure is treated as fresh rather than as part of a crash loop.
	workerHealthyRunTime = time.Minute
	// jobLookupMaxRetries/jobLookupRetryInterval bound how long a worker keeps
	// redelivering an entry whose qor_job rows it cannot find, before treating
	// them as genuinely gone rather than not yet committed.
	jobLookupMaxRetries    = 5
	jobLookupRetryInterval = 3 * time.Second
)

type goque struct {
	q  que.Queue
	db *gorm.DB
	mu sync.Mutex
	// wks holds the currently running workers, replaced as supervisors restart
	// them. Guarded by mu.
	wks []*que.Worker
	// stopCh is closed by Shutdown to stop the supervisors started by the
	// matching Listen. Listen installs a fresh one, so re-enabling the queues
	// after a Shutdown never revives the previous generation. Guarded by mu.
	stopCh chan struct{}
}

// NewGoQueQueue creates a new go-que based Queue (default queue implementation).
// Does not run migrations - call worker.AutoMigrate() first.
func NewGoQueQueue(db *gorm.DB) Queue {
	return newGoQueQueue(db)
}

// newGoQueQueue creates a Queue without migrations.
func newGoQueQueue(db *gorm.DB) Queue {
	if db == nil {
		panic("db can not be nil")
	}

	rdb, err := db.DB()
	if err != nil {
		panic(err)
	}

	// Always disable auto-migration in queue creation
	// Migration is handled by AutoMigrate() function
	q, err := pg.NewWithOptions(pg.Options{
		DB:        rdb,
		DBMigrate: false,
	})
	if err != nil {
		panic(err)
	}

	return &goque{
		q:  q,
		db: db,
	}
}

func (q *goque) Add(ctx context.Context, job QueJobInterface) error {
	jobInfo, err := job.GetJobInfo()
	if err != nil {
		return err
	}
	runAt := time.Now()
	if scheduler, ok := jobInfo.Argument.(Scheduler); ok && scheduler.GetScheduleTime() != nil {
		runAt = scheduler.GetScheduleTime().In(time.Local)
		job.SetStatus(JobStatusScheduled)
	}

	_, err = q.q.Enqueue(ctx, nil, que.Plan{
		Queue: "worker_" + jobInfo.JobName,
		Args:  que.Args(jobInfo.JobID, jobInfo.Argument),
		RunAt: runAt,
	})
	if err != nil {
		return err
	}

	return nil
}

func (*goque) run(ctx context.Context, job QueJobInterface) error {
	job.StartRefresh()
	defer job.StopRefresh()

	return job.GetHandler()(ctx, job)
}

func (*goque) Kill(ctx context.Context, job QueJobInterface) error {
	return job.SetStatus(JobStatusKilled)
}

func (*goque) Remove(ctx context.Context, job QueJobInterface) error {
	return job.SetStatus(JobStatusCancelled)
}

func (q *goque) Listen(jobDefs []*QorJobDefinition, getJob func(qorJobID uint) (QueJobInterface, error)) error {
	q.mu.Lock()
	q.stopCh = make(chan struct{})
	stopCh := q.stopCh
	q.mu.Unlock()

	for i := range jobDefs {
		jd := jobDefs[i]
		if jd.Handler == nil {
			panic(fmt.Sprintf("job %s handler is nil", jd.Name))
		}
		build := func() (*que.Worker, error) {
			return que.NewWorker(que.WorkerOptions{
				Queue:                     "worker_" + jd.Name,
				Mutex:                     q.q.Mutex(),
				MaxLockPerSecond:          10,
				MaxBufferJobsCount:        0,
				MaxPerformPerSecond:       float64(2 * jd.Concurrency),
				MaxConcurrentPerformCount: jd.Concurrency,
				Perform: func(ctx context.Context, qj que.Job) (err error) {
					var job QueJobInterface
					{
						var sid string
						err = q.parseArgs(qj.Plan().Args, &sid)
						if err != nil {
							return err
						}
						id, err := strconv.Atoi(sid)
						if err != nil {
							return err
						}
						job, err = getJob(uint(id))
						if err != nil {
							// The qor_job rows this entry points at are written on
							// another connection, so a lookup can miss one that is
							// merely not committed yet. Plans carry no retry policy,
							// which makes every returned error final — and an expired
							// entry is never redelivered, leaving the instance at
							// "new" forever. Give the writer a few seconds first.
							if qj.RetryCount() < jobLookupMaxRetries {
								if rerr := qj.RetryAfter(ctx, jobLookupRetryInterval, err); rerr == nil {
									return nil
								}
							}
							return err
						}
					}

					defer func() {
						if r := recover(); r != nil {
							job.AddLog(string(debug.Stack()))
							job.SetProgressText(fmt.Sprint(r))
							job.SetStatus(JobStatusException)
							panic(r)
						}
					}()

					if job.GetStatus() == JobStatusCancelled {
						return qj.Expire(ctx, errors.New("job is cancelled"))
					}
					if job.GetStatus() != JobStatusNew && job.GetStatus() != JobStatusScheduled {
						job.SetStatus(JobStatusKilled)
						return errors.New("invalid job status, current status: " + job.GetStatus())
					}

					err = job.SetStatus(JobStatusRunning)
					if err != nil {
						return err
					}

					hctx, cf := context.WithCancel(ctx)
					hDoneC := make(chan struct{})
					isAborted := false
					go func() {
						timer := time.NewTicker(time.Second)
						for {
							select {
							case <-hDoneC:
								return
							case <-timer.C:
								status, _ := job.FetchAndSetStatus()
								if status == JobStatusKilled {
									isAborted = true
									cf()
									return
								}
							}
						}
					}()
					err = q.run(hctx, job)
					if !isAborted {
						hDoneC <- struct{}{}
					}
					if err != nil {
						job.SetProgressText(err.Error())
						job.SetStatus(JobStatusException)
						return err
					}
					if isAborted {
						return qj.Expire(ctx, errors.New("manually aborted"))
					}

					err = job.SetStatus(JobStatusDone)
					if err != nil {
						return err
					}
					return qj.Done(ctx)
				},
			})
		}
		// A worker that cannot even be built is a configuration error, so keep
		// the original fail-fast behaviour for the first one; a later rebuild
		// failing is a runtime problem the supervisor retries instead.
		wk, err := build()
		if err != nil {
			panic(err)
		}
		go q.supervise(jd.Name, wk, build, stopCh)
	}

	return nil
}

// supervise runs wk until it returns, then rebuilds and restarts it with
// exponential backoff, until stopCh is closed by Shutdown. go-que's Run()
// gives up permanently on error (typically a dropped connection), so without
// this the queue would go silently unserved.
func (q *goque) supervise(name string, wk *que.Worker, build func() (*que.Worker, error), stopCh chan struct{}) {
	backoff := workerRestartInitialBackoff
	for {
		if wk != nil {
			q.addWorker(wk)
			started := time.Now()
			err := wk.Run()
			q.removeWorker(wk)
			// Stop() makes Run() return; that is Shutdown's doing, not a failure.
			if isClosed(stopCh) {
				return
			}
			if err != nil {
				q.recordError(fmt.Sprintf("worker %s Run() error: %s", name, err.Error()))
			} else {
				q.recordError(fmt.Sprintf("worker %s Run() returned without error, restarting", name))
			}
			// A worker that served for a while and then died is not a crash
			// loop — let it come back promptly.
			if time.Since(started) >= workerHealthyRunTime {
				backoff = workerRestartInitialBackoff
			}
		}

		select {
		case <-stopCh:
			return
		case <-time.After(backoff):
		}
		if backoff = backoff * 2; backoff > workerRestartMaxBackoff {
			backoff = workerRestartMaxBackoff
		}

		var err error
		if wk, err = build(); err != nil {
			q.recordError(fmt.Sprintf("worker %s rebuild error: %s", name, err.Error()))
			wk = nil
		}
	}
}

// recordError persists a worker failure. Best-effort: these failures are
// usually themselves database problems, so a failed insert is dropped rather
// than masking the original error.
func (q *goque) recordError(msg string) {
	q.db.Create(&GoQueError{Error: msg})
}

func (q *goque) addWorker(wk *que.Worker) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.wks = append(q.wks, wk)
}

func (q *goque) removeWorker(wk *que.Worker) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, w := range q.wks {
		if w == wk {
			q.wks = append(q.wks[:i], q.wks[i+1:]...)
			return
		}
	}
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (q *goque) Shutdown(ctx context.Context) error {
	// Close first: supervisors must see the stop before Stop() unblocks their
	// Run(), or they would treat the shutdown as a failure and restart.
	q.mu.Lock()
	if q.stopCh != nil && !isClosed(q.stopCh) {
		close(q.stopCh)
	}
	wks := append([]*que.Worker(nil), q.wks...)
	q.wks = nil
	q.mu.Unlock()

	var errs error
	for _, wk := range wks {
		if err := wk.Stop(ctx); err != nil {
			errs = multierr.Append(errs, err)
		}
	}
	return errs
}

func (*goque) parseArgs(data []byte, args ...interface{}) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if _, err := d.Token(); err != nil {
		return err
	}
	for _, arg := range args {
		if err := d.Decode(arg); err != nil {
			return err
		}
	}
	return nil
}

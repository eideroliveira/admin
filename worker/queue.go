package worker

import "context"

//go:generate moq -pkg mock -out mock/queue.go . Queue

type QorJobDefinition struct {
	Name        string
	Handler     JobHandler
	Concurrency int
	// OnInterrupted, when set, is called for an instance the queue finds still
	// "running" when it delivers it — the process that was performing it died
	// (a deploy, a crash) and the queue is handing the task to a new one. The
	// instance has already been marked killed; the callback decides what
	// happens next (re-queue it, tell someone). Optional.
	OnInterrupted InterruptedHandler
}

// InterruptedHandler is called for a job instance interrupted by the death of
// the process running it. See QorJobDefinition.OnInterrupted.
type InterruptedHandler func(ctx context.Context, job QueJobInterface)

type Queue interface {
	Add(ctx context.Context, job QueJobInterface) error
	Kill(ctx context.Context, job QueJobInterface) error
	Remove(ctx context.Context, job QueJobInterface) error
	Listen(jobDefs []*QorJobDefinition, getJob func(qorJobID uint) (QueJobInterface, error)) error
	Shutdown(ctx context.Context) error
}

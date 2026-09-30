package worker_test

import (
	"context"
	"testing"

	"github.com/qor5/admin/v3/worker"
)

// acceptingQueue takes every enqueue and runs nothing.
type acceptingQueue struct{ failingQueue }

func (q *acceptingQueue) Add(context.Context, worker.QueJobInterface) error { return nil }

type operatorKey struct{}

// TestCreateSystemJobRecordsTheOperator: CreateSystemJob carries a context and
// no request, which GetCurrentUserIDFunc cannot see, so every job created
// through it — including the ones an admin button starts — used to be stored
// with no operator.
func TestCreateSystemJobRecordsTheOperator(t *testing.T) {
	db := openTestDB(t)
	b := worker.NewWithQueue(db, &acceptingQueue{}).
		OperatorFunc(func(ctx context.Context) string {
			s, _ := ctx.Value(operatorKey{}).(string)
			return s
		})
	b.NewJob("Test Job").Resource(&testArg{})

	ctx := context.WithValue(context.Background(), operatorKey{}, "admin@example.com")
	j, err := b.CreateSystemJob(ctx, "Test Job", &testArg{Name: "x"})
	if err != nil {
		t.Fatalf("CreateSystemJob: %v", err)
	}

	var inst worker.QorJobInstance
	if err := db.Where("qor_job_id = ?", j.ID).First(&inst).Error; err != nil {
		t.Fatalf("load instance: %v", err)
	}
	if inst.Operator != "admin@example.com" {
		t.Fatalf("Operator = %q, want %q", inst.Operator, "admin@example.com")
	}
}

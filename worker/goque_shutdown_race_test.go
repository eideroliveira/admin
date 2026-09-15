package worker

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestGoQueShutdownRightAfterListenLeavesNoWorker pins the race between Listen
// and Shutdown. Listen starts each queue's supervisor in its own goroutine, and
// Shutdown stops only the workers already registered when it runs. A
// supervisor scheduled after that must not register and run its worker anyway,
// or the worker keeps running with nothing left to stop it — in gordpress,
// while the database pools are closed underneath it.
func TestGoQueShutdownRightAfterListenLeavesNoWorker(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	// Many queues, so several supervisors are still unscheduled when Shutdown
	// runs straight after Listen returns.
	const queues = 32
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	var defs []*QorJobDefinition
	for i := 0; i < queues; i++ {
		defs = append(defs, &QorJobDefinition{
			Name:        fmt.Sprintf("shutdownRace%d_%s", i, suffix),
			Handler:     func(context.Context, QorJobInterface) error { return nil },
			Concurrency: 1,
		})
	}

	q := NewGoQueQueue(db).(*goque)
	if err := q.Listen(defs, func(uint) (QueJobInterface, error) {
		return nil, fmt.Errorf("no job expected")
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := q.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// Stop whatever a buggy supervisor registered late, so the test does not
	// leave workers running against the shared database.
	t.Cleanup(func() { _ = q.Shutdown(context.Background()) })

	// Give every supervisor goroutine time to be scheduled.
	time.Sleep(500 * time.Millisecond)

	q.mu.Lock()
	late := len(q.wks)
	q.mu.Unlock()
	if late != 0 {
		t.Fatalf("%d of %d workers were registered after Shutdown returned", late, queues)
	}
}

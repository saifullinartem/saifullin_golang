package positions

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type testPusher struct {
	calls   chan uuid.UUID
	release chan struct{}
	fail    bool
}

func (p *testPusher) RequestPosition(ctx context.Context, _, id uuid.UUID) error {
	if p.calls != nil {
		p.calls <- id
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if p.fail {
		p.fail = false
		return errors.New("push failed")
	}
	return nil
}

func receiveTrip(t *testing.T, calls <-chan uuid.UUID) uuid.UUID {
	t.Helper()
	select {
	case id := <-calls:
		return id
	case <-time.After(time.Second):
		t.Fatal("worker did not process task")
		return uuid.Nil
	}
}

func TestPoolDeduplicatesAndCancels(t *testing.T) {
	pusher := &testPusher{calls: make(chan uuid.UUID, 2)}
	p := New(1, 2, time.Second, pusher, slog.Default())
	trip := model.Trip{ID: uuid.New(), DriverID: uuid.New()}
	if !p.Enqueue(trip) || p.Enqueue(trip) {
		t.Fatal("unexpected enqueue result")
	}
	p.Cancel(trip.ID)
	p.Start(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pusher.calls:
		t.Fatal("cancelled task was pushed")
	default:
	}
}

func TestPoolProcessesWithFixedWorkers(t *testing.T) {
	pusher := &testPusher{calls: make(chan uuid.UUID, 3), release: make(chan struct{})}
	p := New(2, 3, time.Minute, pusher, nil)
	trips := make(map[uuid.UUID]model.Trip)
	for range 3 {
		trip := model.Trip{ID: uuid.New()}
		trips[trip.ID] = trip
		p.Enqueue(trip)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)
	first := receiveTrip(t, pusher.calls)
	second := receiveTrip(t, pusher.calls)
	if first == second || p.EnqueueResult(trips[first]) != Duplicate || p.Size() != 1 {
		t.Fatal("expected two distinct in-flight tasks and one waiting task")
	}
	close(pusher.release)
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	third := receiveTrip(t, pusher.calls)
	if third == first || third == second {
		t.Fatal("duplicate push")
	}
}

func TestPoolContinuesAfterFailureInPriorityOrder(t *testing.T) {
	pusher := &testPusher{calls: make(chan uuid.UUID, 3), fail: true}
	p := New(1, 3, time.Second, pusher, nil)
	var ordered [3]uuid.UUID
	for _, i := range []int{2, 0, 1} {
		ordered[i] = uuid.New()
		p.Enqueue(model.Trip{ID: ordered[i], StartedAt: time.Unix(int64(i), 0)})
	}
	p.Start(t.Context())
	if err := p.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, want := range ordered {
		if got := receiveTrip(t, pusher.calls); got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPoolShutdownCancelsRunningRequest(t *testing.T) {
	pusher := &testPusher{calls: make(chan uuid.UUID, 2), release: make(chan struct{})}
	p := New(1, 2, time.Hour, pusher, nil)
	p.Enqueue(model.Trip{ID: uuid.New()})
	p.Start(t.Context())
	receiveTrip(t, pusher.calls)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := p.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestPoolQueueDropsWithoutBlocking(t *testing.T) {
	p := New(1, 1, time.Second, &testPusher{}, slog.Default())
	if !p.Enqueue(model.Trip{ID: uuid.New(), StartedAt: time.Now().Add(-time.Hour)}) {
		t.Fatal("first enqueue failed")
	}
	started := time.Now()
	if p.Enqueue(model.Trip{ID: uuid.New(), StartedAt: time.Now()}) {
		t.Fatal("full queue accepted task")
	}
	if time.Since(started) > time.Second {
		t.Fatal("enqueue blocked")
	}
}

func TestPoolPrioritizesOlderTripAndEvictsFreshTrip(t *testing.T) {
	p := New(1, 2, time.Second, &testPusher{}, slog.Default())
	oldest := model.Trip{ID: uuid.New(), StartedAt: time.Now().Add(-3 * time.Hour)}
	old := model.Trip{ID: uuid.New(), StartedAt: time.Now().Add(-2 * time.Hour)}
	fresh := model.Trip{ID: uuid.New(), StartedAt: time.Now().Add(-time.Hour)}
	p.Enqueue(old)
	p.Enqueue(fresh)
	if p.Enqueue(oldest) == false || p.Size() != 2 {
		t.Fatal("oldest task was not admitted")
	}
	if _, ok := p.queued[fresh.ID]; ok {
		t.Fatal("fresh task was not evicted")
	}
}

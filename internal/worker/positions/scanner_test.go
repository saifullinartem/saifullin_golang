package positions

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type scanReader struct {
	trips  []model.Trip
	calls  int
	called chan struct{}
}

func (r *scanReader) StaleTripsBatch(_ context.Context, _ time.Duration, limit int, cursor *model.StaleCursor) ([]model.Trip, *model.StaleCursor, error) {
	r.calls++
	if r.called != nil {
		r.called <- struct{}{}
	}
	start := 0
	if cursor != nil {
		for i, trip := range r.trips {
			if trip.ID == cursor.ID {
				start = i + 1
				break
			}
		}
	}
	batch := r.trips[start:min(start+limit, len(r.trips))]
	if len(batch) == 0 {
		return nil, nil, nil
	}
	return batch, &model.StaleCursor{ID: batch[len(batch)-1].ID}, nil
}

type fakeTicker struct{ ch chan time.Time }

func (t *fakeTicker) C() <-chan time.Time { return t.ch }
func (t *fakeTicker) Stop()               {}

func TestScannerUsesOneTickAtATime(t *testing.T) {
	reader := &scanReader{trips: []model.Trip{{ID: uuid.New()}}, called: make(chan struct{}, 1)}
	p := New(1, 2, time.Second, &testPusher{}, slog.Default())
	ticker := &fakeTicker{ch: make(chan time.Time, 1)}
	scanner := NewScanner(reader, p, time.Hour, time.Minute, slog.Default())
	scanner.newTicker = func(time.Duration) Ticker { return ticker }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { scanner.Run(ctx); close(done) }()
	ticker.ch <- time.Now()
	select {
	case <-reader.called:
	case <-time.After(time.Second):
		t.Fatal("scanner did not tick")
	}
	cancel()
	<-done
	if reader.calls != 1 {
		t.Fatalf("scan calls=%d", reader.calls)
	}
}

type scanMetrics struct {
	rows    []int
	results []string
}

func (m *scanMetrics) Stale(result string)         { m.results = append(m.results, result) }
func (*scanMetrics) Request(string, time.Duration) {}
func (m *scanMetrics) RowsRead(count int)          { m.rows = append(m.rows, count) }

func TestScannerBatches(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		capacity, calls, read, size int
		last                        EnqueueResult
	}{
		{"full scan", 250, 3, 250, 250, Queued},
		{"first drop stops scan", 50, 1, 100, 50, Dropped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &scanReader{trips: make([]model.Trip, 250)}
			for i := range reader.trips {
				reader.trips[i] = model.Trip{ID: uuid.New(), StartedAt: time.Unix(int64(i), 0)}
			}
			metrics := &scanMetrics{}
			pool := New(1, tc.capacity, time.Second, &testPusher{}, nil, metrics)
			scanner := NewScanner(reader, pool, time.Hour, time.Minute, nil, 100)
			scanner.scan(t.Context())
			if reader.calls != tc.calls || pool.Size() != tc.size {
				t.Fatalf("queries=%d, queued=%d", reader.calls, pool.Size())
			}
			if len(metrics.rows) != 1 || metrics.rows[0] != tc.read || metrics.results[len(metrics.results)-1] != string(tc.last) {
				t.Fatalf("rows=%v, last result=%v", metrics.rows, metrics.results[len(metrics.results)-1])
			}
			before := len(metrics.results)
			scanner.scan(t.Context())
			if pool.Size() != tc.size {
				t.Fatalf("second tick changed queue size to %d", pool.Size())
			}
			if metrics.results[before] != string(Duplicate) {
				t.Fatal("second tick failed to deduplicate first trip")
			}
		})
	}
}

package telemetry

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Clock struct{}

func (Clock) Now() time.Time { return time.Now() }

type Events struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

type PositionMetrics struct {
	stale    metric.Int64Counter
	requests metric.Int64Counter
	duration metric.Float64Histogram
	rows     metric.Int64Histogram
	saved    metric.Int64Counter
	queue    atomic.Int64
	active   atomic.Int64
}

func NewPositionMetrics(m metric.Meter) (*PositionMetrics, error) {
	stale, err := m.Int64Counter("tripgo.stale_checks")
	if err != nil {
		return nil, err
	}
	requests, err := m.Int64Counter("tripgo.position_requests")
	if err != nil {
		return nil, err
	}
	duration, err := m.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	rows, err := m.Int64Histogram("tripgo.stale_checks.rows_read")
	if err != nil {
		return nil, err
	}
	saved, err := m.Int64Counter("tripgo.positions.saved")
	if err != nil {
		return nil, err
	}
	result := &PositionMetrics{stale: stale, requests: requests, duration: duration, rows: rows, saved: saved}
	_, _ = m.Int64ObservableGauge("tripgo.position_requests.queue.size", metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error { o.Observe(result.queue.Load()); return nil }))
	_, _ = m.Int64ObservableGauge("tripgo.position_requests.workers.active", metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error { o.Observe(result.active.Load()); return nil }))
	return result, nil
}

func (m *PositionMetrics) Stale(result string) {
	m.stale.Add(context.Background(), 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *PositionMetrics) Request(result string, d time.Duration) {
	ctx := context.Background()
	attrs := metric.WithAttributes(attribute.String("result", result))
	m.requests.Add(ctx, 1, attrs)
	if result != "cancelled" {
		m.duration.Record(ctx, d.Seconds(), attrs)
	}
}
func (m *PositionMetrics) Saved()                  { m.saved.Add(context.Background(), 1) }
func (m *PositionMetrics) QueueSize(value int)     { m.queue.Store(int64(value)) }
func (m *PositionMetrics) WorkersActive(delta int) { m.active.Add(int64(delta)) }
func (m *PositionMetrics) RowsRead(value int)      { m.rows.Record(context.Background(), int64(value)) }

func NewEvents(m metric.Meter) (*Events, error) {
	c, err := m.Int64Counter("tripgo.trips.created")
	if err != nil {
		return nil, fmt.Errorf("created counter: %w", err)
	}
	f, err := m.Int64Counter("tripgo.trips.completed")
	if err != nil {
		return nil, fmt.Errorf("completed counter: %w", err)
	}
	return &Events{c, f}, nil
}
func (e *Events) Created(ctx context.Context)   { e.created.Add(ctx, 1) }
func (e *Events) Completed(ctx context.Context) { e.completed.Add(ctx, 1) }

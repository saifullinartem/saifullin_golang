package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/metric"
)

type Clock struct{}

func (Clock) Now() time.Time { return time.Now() }

type Events struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

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

package positions

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type Pusher interface {
	RequestPosition(context.Context, uuid.UUID, uuid.UUID) error
}
type Metrics interface {
	Stale(string)
	Request(string, time.Duration)
}
type queueMetrics interface {
	QueueSize(int)
	WorkersActive(int)
}
type (
	rowsMetrics   interface{ RowsRead(int) }
	EnqueueResult string
)

const (
	Queued    EnqueueResult = "queued"
	Duplicate EnqueueResult = "duplicate"
	Dropped   EnqueueResult = "dropped"
)

type Pool struct {
	queue    []model.Trip
	capacity int
	mu       sync.Mutex
	queued   map[uuid.UUID]struct{}
	links    map[uuid.UUID]trace.SpanContext
	workers  int
	timeout  time.Duration
	pusher   Pusher
	logger   *slog.Logger
	wg       sync.WaitGroup
	metrics  Metrics
	stop     chan struct{}
	stopOnce sync.Once
	cancel   context.CancelFunc
	done     chan struct{}
	notify   chan struct{}
	tracer   trace.Tracer
}

func New(workers, size int, timeout time.Duration, pusher Pusher, logger *slog.Logger, metrics ...Metrics) *Pool {
	p := &Pool{capacity: size, queued: make(map[uuid.UUID]struct{}), links: make(map[uuid.UUID]trace.SpanContext), workers: workers, timeout: timeout, pusher: pusher, logger: logger, stop: make(chan struct{}), notify: make(chan struct{}, 1), done: make(chan struct{})}
	if len(metrics) > 0 {
		p.metrics = metrics[0]
	}
	return p
}

func (p *Pool) Start(ctx context.Context) {
	if p.logger != nil {
		p.logger.InfoContext(ctx, "position workers started", "workers", p.workers)
	}
	ctx, p.cancel = context.WithCancel(ctx)
	for range p.workers {
		p.wg.Go(func() { p.worker(ctx) })
	}
	go func() {
		p.wg.Wait()
		if p.logger != nil {
			p.logger.InfoContext(ctx, "position workers stopped")
		}
		close(p.done)
	}()
}

func staleAt(t model.Trip) time.Time {
	if t.LastPositionAt != nil {
		return *t.LastPositionAt
	}
	return t.StartedAt
}
func (p *Pool) Enqueue(t model.Trip) bool { return p.EnqueueResult(t) == Queued }
func (p *Pool) EnqueueResult(t model.Trip) EnqueueResult {
	return p.EnqueueWithLink(t, trace.SpanContext{})
}

func (p *Pool) EnqueueWithLink(t model.Trip, link trace.SpanContext) EnqueueResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.queued[t.ID]; ok {
		p.recordStale(Duplicate)
		return Duplicate
	}
	select {
	case <-p.stop:
		return Dropped
	default:
	}
	if len(p.queue) == p.capacity {
		newest := p.queue[len(p.queue)-1]
		if compareTrips(t, newest) >= 0 {
			p.recordDrop(t.ID)
			return Dropped
		}
		delete(p.queued, newest.ID)
		delete(p.links, newest.ID)
		p.queue = p.queue[:len(p.queue)-1]
		p.recordDrop(newest.ID)
	}
	index, _ := slices.BinarySearchFunc(p.queue, t, compareTrips)
	p.queue = slices.Insert(p.queue, index, t)
	p.queued[t.ID] = struct{}{}
	p.links[t.ID] = link
	p.recordStale(Queued)
	p.recordSize()
	p.signal()
	return Queued
}

func compareTrips(a, b model.Trip) int {
	if order := staleAt(a).Compare(staleAt(b)); order != 0 {
		return order
	}
	return bytes.Compare(a.ID[:], b.ID[:])
}

func (p *Pool) recordDrop(id uuid.UUID) {
	p.recordStale(Dropped)
	if p.logger != nil {
		p.logger.Warn("position request dropped: queue full", "trip_id", id)
	}
}

func (p *Pool) recordStale(result EnqueueResult) {
	if p.metrics != nil {
		p.metrics.Stale(string(result))
	}
}

func (p *Pool) recordSize() {
	if metrics, ok := p.metrics.(queueMetrics); ok {
		metrics.QueueSize(len(p.queue))
	}
}

func (p *Pool) signal() {
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

func (p *Pool) Cancel(id uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, t := range p.queue {
		if t.ID == id {
			p.queue = slices.Delete(p.queue, i, i+1)
			delete(p.queued, id)
			delete(p.links, id)
			p.recordSize()
			if p.metrics != nil {
				p.metrics.Request("cancelled", 0)
			}
			return
		}
	}
}

func (p *Pool) next() (model.Trip, trace.SpanContext, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return model.Trip{}, trace.SpanContext{}, false
	}
	t := p.queue[0]
	p.queue = slices.Delete(p.queue, 0, 1)
	link := p.links[t.ID]
	delete(p.links, t.ID)
	p.recordSize()
	if len(p.queue) != 0 {
		p.signal()
	}
	return t, link, true
}

func (p *Pool) worker(ctx context.Context) {
	for ctx.Err() == nil {
		trip, link, ok := p.next()
		if ok {
			p.process(ctx, trip, link)
			continue
		}
		select {
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		case <-p.notify:
		}
	}
}

func (p *Pool) process(ctx context.Context, t model.Trip, link trace.SpanContext) {
	defer func() {
		p.mu.Lock()
		delete(p.queued, t.ID)
		p.mu.Unlock()
	}()
	if p.tracer != nil {
		var span trace.Span
		if link.IsValid() {
			ctx, span = p.tracer.Start(ctx, "position.worker", trace.WithLinks(trace.Link{SpanContext: link}))
		} else {
			ctx, span = p.tracer.Start(ctx, "position.worker")
		}
		defer span.End()
	}
	started := time.Now()
	if m, ok := p.metrics.(queueMetrics); ok {
		m.WorkersActive(1)
	}
	taskCtx, cancel := context.WithTimeout(ctx, p.timeout)
	err := p.pusher.RequestPosition(taskCtx, t.DriverID, t.ID)
	cancel()
	if p.metrics != nil {
		result := "success"
		if err != nil {
			result = "error"
		}
		p.metrics.Request(result, time.Since(started))
		if m, ok := p.metrics.(queueMetrics); ok {
			m.WorkersActive(-1)
		}
	}
	if err != nil && p.logger != nil {
		p.logger.Warn("position request failed", "trip_id", t.ID, "error", err)
	}
}
func (p *Pool) SetTracer(tracer trace.Tracer) { p.tracer = tracer }

func (p *Pool) Size() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.queue) }

func (p *Pool) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		close(p.stop)
		p.mu.Unlock()
	})
	stopDeadline := context.AfterFunc(ctx, p.cancel)
	defer stopDeadline()
	<-p.done
	p.cancel()
	return ctx.Err()
}

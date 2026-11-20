package positions

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type BatchReader interface {
	StaleTripsBatch(context.Context, time.Duration, int, *model.StaleCursor) ([]model.Trip, *model.StaleCursor, error)
}
type Ticker interface {
	C() <-chan time.Time
	Stop()
}
type (
	tickerFactory func(time.Duration) Ticker
	realTicker    struct{ *time.Ticker }
)

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

type Scanner struct {
	reader          BatchReader
	pool            *Pool
	interval, stale time.Duration
	logger          *slog.Logger
	newTicker       tickerFactory
	batchSize       int
	tracer          trace.Tracer
}

func NewScanner(reader BatchReader, pool *Pool, interval, stale time.Duration, logger *slog.Logger, batchSize ...int) *Scanner {
	s := &Scanner{reader: reader, pool: pool, interval: interval, stale: stale, logger: logger, newTicker: func(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }, batchSize: 100}
	if len(batchSize) > 0 && batchSize[0] > 0 {
		s.batchSize = batchSize[0]
	}
	return s
}
func (s *Scanner) SetTracer(tracer trace.Tracer) { s.tracer = tracer }

func (s *Scanner) Run(ctx context.Context) {
	if s.logger != nil {
		s.logger.InfoContext(ctx, "position scanner started")
		defer s.logger.InfoContext(ctx, "position scanner stopped")
	}
	ticker := s.newTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			s.scan(ctx)
		}
	}
}

func (s *Scanner) scan(ctx context.Context) {
	var scanSpan trace.Span
	if s.tracer != nil {
		ctx, scanSpan = s.tracer.Start(ctx, "positions.scan")
		defer scanSpan.End()
	}
	var cursor *model.StaleCursor
	rowsRead := 0
	if metrics, ok := s.pool.metrics.(rowsMetrics); ok {
		defer func() { metrics.RowsRead(rowsRead) }()
	}
	for ctx.Err() == nil {
		trips, next, err := s.reader.StaleTripsBatch(ctx, s.stale, s.batchSize, cursor)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("stale trips scan failed", "error", err)
			}
			return
		}
		rowsRead += len(trips)
		for _, t := range trips {
			if ctx.Err() != nil || s.pool.EnqueueWithLink(t, spanContext(scanSpan)) == Dropped {
				return
			}
		}
		if len(trips) < s.batchSize || next == nil {
			return
		}
		cursor = next
	}
}

func spanContext(span trace.Span) trace.SpanContext {
	if span == nil {
		return trace.SpanContext{}
	}
	return span.SpanContext()
}

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type QueryTracer struct {
	tracer   *otelpgx.Tracer
	duration metric.Float64Histogram
}
type (
	queryKey   struct{}
	queryState struct {
		started    time.Time
		operation  string
		collection string
	}
)

func NewTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*QueryTracer, error) {
	d, err := mp.Meter("tripgo/postgres").Float64Histogram("db.client.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5))
	if err != nil {
		return nil, fmt.Errorf("db duration: %w", err)
	}
	return &QueryTracer{otelpgx.NewTracer(otelpgx.WithTracerProvider(tp), otelpgx.WithMeterProvider(noop.NewMeterProvider()), otelpgx.WithDisableSQLStatementInAttributes(), otelpgx.WithDisableConnectionDetailsInAttributes()), d}, nil
}

func (t *QueryTracer) TraceQueryStart(ctx context.Context, c *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.tracer.TraceQueryStart(ctx, c, data)
	op, table := queryLabels(data.SQL)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.operation.name", op), attribute.String("db.collection.name", table))
	return context.WithValue(ctx, queryKey{}, queryState{time.Now(), op, table})
}

func (t *QueryTracer) TraceQueryEnd(ctx context.Context, c *pgx.Conn, data pgx.TraceQueryEndData) {
	state, _ := ctx.Value(queryKey{}).(queryState)
	errorType := ""
	if data.Err != nil {
		errorType = "database_error"
		var pe *pgconn.PgError
		if errors.As(data.Err, &pe) {
			errorType = pe.Code
		}
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, errorType)
		span.SetAttributes(attribute.String("error.type", errorType))
	}
	t.duration.Record(ctx, time.Since(state.started).Seconds(), metric.WithAttributes(attribute.String("db.operation.name", state.operation), attribute.String("db.collection.name", state.collection), attribute.String("error.type", errorType)))
	t.tracer.TraceQueryEnd(ctx, c, data)
}

func queryLabels(sql string) (string, string) {
	parts := strings.Fields(sql)
	if len(parts) == 0 {
		return "OTHER", ""
	}
	op := strings.ToUpper(parts[0])
	table := ""
	for i, p := range parts {
		if (strings.EqualFold(p, "INTO") || strings.EqualFold(p, "FROM") || strings.EqualFold(p, "UPDATE")) && i+1 < len(parts) {
			candidate := strings.Trim(parts[i+1], "\"")
			if candidate == "trips" || candidate == "trip_status_history" {
				table = candidate
				break
			}
		}
	}
	switch op {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "BEGIN", "COMMIT", "ROLLBACK":
	default:
		op = "OTHER"
	}
	return op, table
}

package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/response"
	"github.com/saifullinartem/saifullin_golang/internal/http/routing"
)

type Middleware struct {
	logger   *slog.Logger
	tp       trace.TracerProvider
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
	router   chi.Router
}

func New(l *slog.Logger, tp trace.TracerProvider, mp metric.MeterProvider, r chi.Router) (*Middleware, error) {
	meter := mp.Meter("tripgo/http")
	d, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10))
	if err != nil {
		return nil, fmt.Errorf("HTTP duration: %w", err)
	}
	a, err := meter.Int64UpDownCounter("http.server.active_requests")
	if err != nil {
		return nil, fmt.Errorf("active requests: %w", err)
	}
	return &Middleware{l, tp, d, a, r}, nil
}

func (m *Middleware) route(r *http.Request) string {
	return routing.Pattern(m.router, r)
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return m.recover(m.tracing(m.logging(m.metrics(next))))
}

func (m *Middleware) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &response.State{Recover: m.serveBuffered}
		next.ServeHTTP(w, r.WithContext(response.WithState(r.Context(), state)))
	})
}

func (m *Middleware) serveBuffered(w http.ResponseWriter, r *http.Request, next http.Handler) {
	buffer := response.NewBuffer()
	func() {
		defer func() {
			if p := recover(); p != nil {
				err := fmt.Errorf("panic: %v", p)
				stack := string(debug.Stack())
				buffer.Reset()
				domainerror.Write(buffer, r, err)
				response.SetPanic(r.Context(), err, stack)
			}
		}()
		next.ServeHTTP(buffer, r)
	}()
	buffer.Commit(w)
}

func (m *Middleware) tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := m.route(r)
		handler := otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := trace.SpanFromContext(r.Context())
			span.SetAttributes(attribute.String("http.route", route))
			next.ServeHTTP(w, r)
			if err := response.Error(r.Context()); err != nil {
				kind := response.ErrorType(r.Context())
				span.SetStatus(codes.Error, kind)
				span.SetAttributes(attribute.String("error.type", kind))
				span.RecordError(err)
			}
		}), r.Method+" "+route, otelhttp.WithTracerProvider(m.tp), otelhttp.WithMeterProvider(noop.NewMeterProvider()), otelhttp.WithPropagators(propagation.TraceContext{}), otelhttp.WithSpanNameFormatter(func(_ string, _ *http.Request) string { return r.Method + " " + route }))
		handler.ServeHTTP(w, r)
	})
}

func (m *Middleware) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			status := rw.Status()
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			}
			args := []any{"method", r.Method, "route", m.route(r), "status", status, "duration_ms", float64(time.Since(start).Microseconds()) / 1000}
			if err := response.Error(r.Context()); err != nil && status >= 500 {
				args = append(args, "error", err)
			}
			if stack := response.Stack(r.Context()); stack != "" {
				args = append(args, "stack", stack)
			}
			m.logger.Log(r.Context(), level, "request handled", args...)
		}()
		next.ServeHTTP(rw, r)
	})
}

func (m *Middleware) metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attrs := []attribute.KeyValue{attribute.String("http.request.method", r.Method), attribute.String("http.route", m.route(r))}
		m.active.Add(r.Context(), 1, metric.WithAttributes(attrs...))
		start := time.Now()
		rw := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			status := rw.Status()
			if status == 0 {
				status = http.StatusOK
			}
			m.active.Add(r.Context(), -1, metric.WithAttributes(attrs...))
			m.duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(append(attrs, attribute.Int("http.response.status_code", status))...))
		}()
		response.Serve(rw, r, next)
	})
}

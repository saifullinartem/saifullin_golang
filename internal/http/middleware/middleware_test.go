package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/saifullinartem/saifullin_golang/api"
	transport "github.com/saifullinartem/saifullin_golang/internal/http"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/middleware"
)

func TestSignals(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		panics  bool
		partial string
	}{{"success", 200, false, ""}, {"client error", 400, false, ""}, {"internal error", 500, false, ""}, {"panic", 500, true, ""}, {"panic after headers", 500, true, "headers"}, {"panic after body", 500, true, "body"}} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			router := chi.NewRouter()
			fixture := newSignals(t, router)
			reader, spans, logs := fixture.reader, fixture.spans, fixture.logs
			router.Get("/api/v1/trips/{tripId}", func(w http.ResponseWriter, r *http.Request) {
				var rm metricdata.ResourceMetrics
				if err := reader.Collect(r.Context(), &rm); err != nil {
					t.Fatal(err)
				}
				if activeValue(rm) != 1 {
					t.Fatal("active_requests must be 1 inside handler")
				}
				if !trace.SpanContextFromContext(r.Context()).IsValid() {
					t.Fatal("handler has no trace context")
				}
				if tt.panics {
					if tt.partial != "" {
						w.Header().Set("Location", "/must-not-leak")
						w.Header().Set("Content-Type", "application/json")
						w.Header().Set("Content-Length", "7")
						w.WriteHeader(http.StatusCreated)
					}
					if tt.partial == "body" {
						_, _ = w.Write([]byte(`{"id":`))
					}
					panic("test panic")
				}
				if tt.status == 500 {
					domainerror.Write(w, r, fmt.Errorf("secret database detail"))
					return
				}
				w.WriteHeader(tt.status)
			})
			r := httptest.NewRequest("GET", "/api/v1/trips/5cb72c04-7650-45c9-a79b-bcdba0631e0c", nil)
			r.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
			w := httptest.NewRecorder()
			fixture.handler.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d,body=%s", w.Code, w.Body)
			}
			if tt.panics {
				assertPanicProblem(t, w.Code, w.Header(), w.Body.Bytes())
			}

			ended := spans.Ended()
			if len(ended) != 1 {
				t.Fatalf("spans=%d", len(ended))
			}
			span := ended[0]
			if span.Name() != "GET /api/v1/trips/{tripId}" || span.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || span.Parent().SpanID().String() != "00f067aa0ba902b7" {
				t.Fatalf("span=%s %+v", span.Name(), span.SpanContext())
			}
			assertSpanStatus(t, span, tt.status)
			if tt.panics {
				assertPanicLog(t, logs.String())
			}
			assertMetrics(ctx, t, reader, tt.status)
			assertLogs(t, logs.String(), tt.status)
		})
	}
}

func activeValue(rm metricdata.ResourceMetrics) int64 {
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "http.server.active_requests" {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok && len(sum.DataPoints) == 1 {
					return sum.DataPoints[0].Value
				}
			}
		}
	}
	return -1
}

func assertMetrics(ctx context.Context, t *testing.T, reader *sdkmetric.ManualReader, wantStatus int) {
	t.Helper()
	assertMetricsRoute(ctx, t, reader, wantStatus, "/api/v1/trips/{tripId}")
}

func assertMetricsRoute(ctx context.Context, t *testing.T, reader *sdkmetric.ManualReader, wantStatus int, wantRoute string) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatal(err)
	}
	if activeValue(rm) != 0 {
		t.Fatal("active_requests did not return to zero")
	}
	found := false
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			if !ok || len(h.DataPoints) != 1 {
				t.Fatalf("histogram=%+v", m)
			}
			p := h.DataPoints[0]
			route, _ := p.Attributes.Value(attribute.Key("http.route"))
			status, _ := p.Attributes.Value(attribute.Key("http.response.status_code"))
			if p.Count != 1 || route.AsString() != wantRoute || int(status.AsInt64()) != wantStatus {
				t.Fatalf("duration point=%+v", p)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("duration metric missing")
	}
}

func assertLogs(t *testing.T, logs string, wantStatus int) {
	t.Helper()
	assertLogsRoute(t, logs, wantStatus, "/api/v1/trips/{tripId}")
}

func assertLogsRoute(t *testing.T, logs string, wantStatus int, wantRoute string) {
	t.Helper()

	count := 0
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["msg"] != "request handled" {
			continue
		}
		count++
		level := "INFO"
		if wantStatus >= 500 {
			level = "ERROR"
		}
		if record["route"] != wantRoute || record["status"] != float64(wantStatus) || record["level"] != level {
			t.Fatalf("request log=%v", record)
		}
	}
	if count != 1 {
		t.Fatalf("request logs=%d", count)
	}
}

type signals struct {
	handler http.Handler
	reader  *sdkmetric.ManualReader
	spans   *tracetest.SpanRecorder
	logs    *bytes.Buffer
}

func newSignals(t *testing.T, router chi.Router) *signals {
	t.Helper()
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := mp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(spans))
	t.Cleanup(func() {
		if err := tp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	logs := &bytes.Buffer{}
	mw, err := middleware.New(slog.New(slog.NewJSONHandler(logs, nil)), tp, mp, router)
	if err != nil {
		t.Fatal(err)
	}
	return &signals{mw.Wrap(router), reader, spans, logs}
}

func assertSpanStatus(t *testing.T, span sdktrace.ReadOnlySpan, status int) {
	t.Helper()
	found := false
	for _, a := range span.Attributes() {
		if a.Key == "http.status_code" || a.Key == "http.response.status_code" {
			found = true
			if int(a.Value.AsInt64()) != status {
				t.Fatalf("span HTTP status=%v,want %d", a, status)
			}
		}
	}
	if !found {
		t.Fatal("span has no HTTP status")
	}
	if status >= 500 && span.Status().Code != codes.Error {
		t.Fatalf("span status=%v", span.Status())
	}
}

func assertPanicProblem(t *testing.T, status int, header http.Header, body []byte) {
	t.Helper()
	var problem api.Problem
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("invalid Problem: %v; body=%s", err, body)
	}
	if status != 500 || problem.Status != 500 || problem.Code != "internal_error" || header.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("panic response=%d %s", status, body)
	}
	if header.Get("Location") != "" || header.Get("Content-Length") == "7" || strings.Contains(string(body), "test panic") || strings.Contains(string(body), `"id"`) {
		t.Fatalf("partial response leaked: %v %s", header, body)
	}
}

func assertPanicLog(t *testing.T, logs string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	if len(lines) != 1 {
		t.Fatalf("panic must produce one request log: %s", logs)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatal(err)
	}
	if record["error"] == nil || record["stack"] == nil || !strings.Contains(fmt.Sprint(record["error"]), "test panic") || !strings.Contains(fmt.Sprint(record["stack"]), "goroutine") {
		t.Fatalf("panic context missing: %v", record)
	}
}

func TestPanicAfterWriteOverHTTP(t *testing.T) {
	router := chi.NewRouter()
	fixture := newSignals(t, router)
	router.Get("/api/v1/trips/{tripId}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/must-not-leak")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":`))
		panic("test panic")
	})
	server := httptest.NewServer(fixture.handler)
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 3 * time.Second
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/v1/trips/id", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	assertPanicProblem(t, result.StatusCode, result.Header, body)
	assertLogs(t, fixture.logs.String(), 500)
	assertPanicLog(t, fixture.logs.String())
	assertMetrics(context.Background(), t, fixture.reader, 500)
	ended := fixture.spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("spans=%d", len(ended))
	}
	assertSpanStatus(t, ended[0], 500)
}

func TestRouterSignals(t *testing.T) {
	for _, tt := range []struct {
		method, path, route string
		status              int
	}{
		{http.MethodGet, "/unknown", "unmatched", 404},
		{http.MethodDelete, "/api/v1/trips/id", "/api/v1/trips/{tripId}", 405},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			handler := transport.New(nil, nil, nil, nil, time.Second)
			router := transport.NewRouter(handler)
			fixture := newSignals(t, router)
			w := httptest.NewRecorder()
			fixture.handler.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code != tt.status {
				t.Fatalf("response=%d %s", w.Code, w.Body)
			}
			assertMetricsRoute(context.Background(), t, fixture.reader, tt.status, tt.route)
			assertLogsRoute(t, fixture.logs.String(), tt.status, tt.route)
			ended := fixture.spans.Ended()
			if len(ended) != 1 {
				t.Fatalf("spans=%d", len(ended))
			}
			if ended[0].Name() != tt.method+" "+tt.route {
				t.Fatalf("span=%s", ended[0].Name())
			}
			assertSpanStatus(t, ended[0], tt.status)
		})
	}
}

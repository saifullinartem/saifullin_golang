package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	transport "github.com/saifullinartem/saifullin_golang/internal/http"
	"github.com/saifullinartem/saifullin_golang/internal/http/middleware"
	"github.com/saifullinartem/saifullin_golang/internal/http/response"
)

const (
	methodNotAllowedCode = "method_not_allowed"
	idempotencyHeader    = "Idempotency-Key"
)

func TestProbes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		body   string
	}{{"ready", nil, 200, "ok"}, {"not ready", errors.New("database down"), 503, "unavailable"}} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := NewMockpinger(ctrl)
			db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("readiness has no timeout")
				}
				return tt.err
			})
			h := transport.New(NewMockcreateHandler(ctrl), NewMockgetHandler(ctrl), NewMockfinishHandler(ctrl), db, time.Second)
			w := httptest.NewRecorder()
			h.Ready(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.body) {
				t.Fatalf("status=%d,body=%s", w.Code, w.Body)
			}
		})
	}
}

func TestHealth(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := transport.New(NewMockcreateHandler(ctrl), NewMockgetHandler(ctrl), NewMockfinishHandler(ctrl), NewMockpinger(ctrl), time.Second)
	w := httptest.NewRecorder()
	h.Health(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "ok") {
		t.Fatalf("health=%s", w.Body)
	}
}

func TestParameterError(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := transport.New(NewMockcreateHandler(ctrl), NewMockgetHandler(ctrl), NewMockfinishHandler(ctrl), NewMockpinger(ctrl), time.Second)
	router := transport.NewRouter(h)
	for _, url := range []string{"/api/v1/trips/invalid", "/api/v1/trips/invalid/finish"} {
		method := http.MethodGet
		if strings.HasSuffix(url, "finish") {
			method = http.MethodPost
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, url, nil))
		if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_request") {
			t.Fatalf("status=%d,body=%s", w.Code, w.Body)
		}
	}
}

func TestRouterProblems(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler := transport.New(NewMockcreateHandler(ctrl), NewMockgetHandler(ctrl), NewMockfinishHandler(ctrl), NewMockpinger(ctrl), time.Second)
	router := transport.NewRouter(handler)
	for _, tt := range []struct {
		method, path string
		status       int
		code, allow  string
	}{
		{http.MethodGet, "/unknown", 404, "not_found", ""},
		{http.MethodPost, "/unknown", 404, "not_found", ""},
		{http.MethodPost, "/health", 405, methodNotAllowedCode, http.MethodGet},
		{http.MethodDelete, "/api/v1/trips/id", 405, methodNotAllowedCode, http.MethodGet},
		{http.MethodGet, "/api/v1/trips", 405, methodNotAllowedCode, http.MethodPost},
		{http.MethodOptions, "/api/v1/trips/id/finish", 405, methodNotAllowedCode, http.MethodPost},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			var problem api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if w.Code != tt.status || int(problem.Status) != tt.status || problem.Code != tt.code || problem.Instance == nil || *problem.Instance != tt.path || w.Header().Get("Content-Type") != "application/problem+json" || w.Header().Get("Allow") != tt.allow {
				t.Fatalf("response=%d %v %s", w.Code, w.Header(), w.Body)
			}
		})
	}
}

func TestParameterDetailsAndCause(t *testing.T) {
	cause := errors.New("invalid UUID format")
	for _, tt := range []struct {
		name          string
		err           error
		field, reason string
	}{
		{"UUID", &api.InvalidParamFormatError{ParamName: "tripId", Err: cause}, "tripId", "must be a valid UUID"},
		{"header UUID", &api.InvalidParamFormatError{ParamName: idempotencyHeader, Err: cause}, idempotencyHeader, "must be a valid UUID"},
		{"multiple header values", &api.TooManyValuesForParamError{ParamName: idempotencyHeader, Count: 2}, idempotencyHeader, "must have a single value"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := &response.State{}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/trips/broken", nil)
			r = r.WithContext(response.WithState(r.Context(), state))
			w := httptest.NewRecorder()
			transport.ParameterError(w, r, tt.err)
			var problem api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			var invalid *trip.ValidationError
			if !errors.As(state.Err, &invalid) || invalid.Field != tt.field || !errors.Is(state.Err, tt.err) || problem.Detail == nil || *problem.Detail != tt.field+": "+tt.reason {
				t.Fatalf("details/cause lost: %v %s", state.Err, w.Body)
			}
			if tt.name != "multiple header values" && !errors.Is(state.Err, cause) {
				t.Fatal("original UUID cause lost")
			}
		})
	}
}

func TestReadinessFailureSpan(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := NewMockpinger(ctrl)
	db.EXPECT().Ping(gomock.Any()).Return(errors.New("database unavailable"))
	handler := transport.New(NewMockcreateHandler(ctrl), NewMockgetHandler(ctrl), NewMockfinishHandler(ctrl), db, time.Second)
	router := transport.NewRouter(handler)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	mw, err := middleware.New(slog.New(slog.NewJSONHandler(io.Discard, nil)), provider, noop.NewMeterProvider(), router)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mw.Wrap(router).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	var health api.HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if w.Code != 503 || health.Status != api.Unavailable {
		t.Fatalf("readiness contract changed: %d %s", w.Code, w.Body)
	}
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("spans=%v", spans)
	}
	found := false
	for _, attr := range spans[0].Attributes() {
		if attr.Key == "error.type" && attr.Value.AsString() == "database_unavailable" {
			found = true
		}
	}
	if !found || len(spans[0].Events()) != 1 {
		t.Fatalf("readiness error not recorded once: %v %v", spans[0].Attributes(), spans[0].Events())
	}
}

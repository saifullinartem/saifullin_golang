package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/saifullinartem/saifullin_golang/internal/infra/telemetry"
)

func TestLogger(t *testing.T) {
	var out bytes.Buffer
	l := telemetry.Logger(&out, "info", "trip-service")
	l.DebugContext(context.Background(), "hidden")
	tid, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	sid, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid}))
	l.With("operation", "create").InfoContext(ctx, "trip created")
	var r map[string]any
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r["trace_id"] != tid.String() || r["span_id"] != sid.String() || r["service"] != "trip-service" || r["operation"] != "create" || r["time"] == nil || strings.Contains(out.String(), "hidden") {
		t.Fatalf("log=%s", out.String())
	}
	out.Reset()
	l.InfoContext(context.Background(), "startup")
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r["trace_id"] != "" || r["span_id"] != "" {
		t.Fatal("startup context has request ids")
	}
}

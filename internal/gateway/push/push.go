package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Gateway struct {
	url    string
	client *http.Client
	tracer trace.Tracer
}

func New(url string, timeout time.Duration, tracers ...trace.Tracer) *Gateway {
	var tracer trace.Tracer
	if len(tracers) > 0 {
		tracer = tracers[0]
	}
	return &Gateway{url: strings.TrimRight(url, "/"), client: &http.Client{Timeout: timeout}, tracer: tracer}
}

func (g *Gateway) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	if g.tracer != nil {
		var span trace.Span
		ctx, span = g.tracer.Start(ctx, "push.request")
		defer span.End()
	}
	body, err := json.Marshal(map[string]any{
		"request_id":   uuid.NewString(),
		"recipient_id": driverID.String(),
		"kind":         "REQUEST_POSITION",
		"data":         map[string]string{"trip_id": tripID.String()},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url+"/api/v1/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("push transport: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return fmt.Errorf("push service client error: status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("push service server error: status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("push service unexpected status %d", resp.StatusCode)
	}
	return nil
}
func (g *Gateway) Close() { g.client.CloseIdleConnections() }

package push

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGatewayRequestPosition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/push" || r.Method != http.MethodPost {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			RequestID   string            `json:"request_id"`
			RecipientID string            `json:"recipient_id"`
			Kind        string            `json:"kind"`
			Data        map[string]string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.RequestID == "" || body.RecipientID == "" || body.Kind != "REQUEST_POSITION" || body.Data["trip_id"] == "" {
			t.Fatalf("body=%+v", body)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	gateway := New(server.URL, time.Second)
	if err := gateway.RequestPosition(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayDistinguishesHTTPFailures(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusServiceUnavailable} {
		t.Run(strings.TrimSpace(http.StatusText(status)), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			err := New(server.URL, time.Second).RequestPosition(context.Background(), uuid.New(), uuid.New())
			if err == nil {
				t.Fatal("expected error")
			}
			if status < 500 && !strings.Contains(err.Error(), "client error") {
				t.Fatalf("error=%v", err)
			}
			if status >= 500 && !strings.Contains(err.Error(), "server error") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

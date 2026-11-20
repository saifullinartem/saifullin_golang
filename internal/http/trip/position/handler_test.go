package position

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRejectsMalformedPositionBody(t *testing.T) {
	for _, body := range []string{
		`{"recorded_at":"2026-11-19T09:00:00Z"}`,
		`{"coordinates":{},"recorded_at":"2026-11-19T09:00:00Z"}`,
		`{"coordinates":{"latitude":0,"longitude":null},"recorded_at":"2026-11-19T09:00:00Z"}`,
		`{"coordinates":{"latitude":0,"longitude":0,"unknown":0},"recorded_at":"2026-11-19T09:00:00Z"}`,
		`{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-11-19T09:00:00Z"} {}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		New(nil).CreateTripPosition(resp, req, uuid.New())
		if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "invalid_request") {
			t.Fatalf("request=%s response=%d %s", body, resp.Code, resp.Body)
		}
	}
}

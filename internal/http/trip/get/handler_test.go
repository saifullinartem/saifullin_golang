package get_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/http/trip/get"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

func TestHandler(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{{"success", nil, 200, ""}, {"not found", trip.ErrTripNotFound, 404, "trip_not_found"}, {"completed", trip.ErrTripCompleted, 409, "trip_completed"}, {"failure", errors.New("secret SQL password"), 500, "internal_error"}} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			u := NewMockusecase(ctrl)
			id := uuid.New()
			at := time.Now().UTC()
			value := model.Trip{ID: id, Status: trip.Completed, StartedAt: at, FinishedAt: &at}
			u.EXPECT().Get(gomock.Any(), id).Return(value, tt.err)
			r := httptest.NewRequest("POST", "/api/v1/trips/"+id.String(), nil)
			w := httptest.NewRecorder()
			get.New(u).GetTrip(w, r, id)
			if w.Code != tt.status {
				t.Fatalf("status=%d,body=%s", w.Code, w.Body)
			}
			if tt.err == nil {
				var got api.Trip
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Id != id || got.Status != api.Completed {
					t.Fatalf("trip=%s", w.Body)
				}
				return
			}
			var p api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if p.Code != tt.code || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("problem=%s", w.Body)
			}
		})
	}
}

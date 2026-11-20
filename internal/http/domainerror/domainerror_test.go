package domainerror_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("get: %w", trip.ErrTripNotFound), 404, "trip_not_found"},
		{fmt.Errorf("finish: %w", trip.ErrTripCompleted), 409, "trip_completed"},
		{fmt.Errorf("create: %w", trip.ErrDriverBusy), 409, "driver_busy"},
		{fmt.Errorf("validate: %w", &trip.ValidationError{Field: "price", Reason: "negative"}), 400, "invalid_request"},
		{errors.New("secret SQL password"), 500, "internal_error"},
		{fmt.Errorf("route: %w", domainerror.ErrRouteNotFound), 404, "not_found"},
		{fmt.Errorf("method: %w", domainerror.ErrMethodNotAllowed), 405, "method_not_allowed"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			status, p := domainerror.Problem(tt.err)
			if status != tt.status || int(p.Status) != status || p.Code != tt.code || p.Title == "" || p.Detail == nil || p.Type != "https://tripgo.example/problems/"+strings.ReplaceAll(tt.code, "_", "-") {
				t.Fatalf("Problem=%d %+v", status, p)
			}
			if tt.status == 500 && strings.Contains(*p.Detail, "secret") {
				t.Fatal("internal details leaked")
			}
		})
	}
}

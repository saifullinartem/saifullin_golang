package domainerror

import (
	"errors"
	"net/http"
	"strings"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/http/response"
)

var (
	ErrRouteNotFound    = errors.New("route not found")
	ErrMethodNotAllowed = errors.New("method not allowed")
)

func Problem(err error) (int, api.Problem) {
	status, code, title, detail := http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error"
	var invalid *trip.ValidationError
	switch {
	case errors.Is(err, ErrRouteNotFound):
		status, code, title, detail = http.StatusNotFound, "not_found", "Not Found", "Route was not found"
	case errors.Is(err, ErrMethodNotAllowed):
		status, code, title, detail = http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", "Method is not allowed for this route"
	case errors.As(err, &invalid):
		status, code, title, detail = http.StatusBadRequest, "invalid_request", "Invalid request", invalid.Error()
	case errors.Is(err, trip.ErrTripNotFound):
		status, code, title, detail = http.StatusNotFound, "trip_not_found", "Trip not found", "Trip was not found"
	case errors.Is(err, trip.ErrTripCompleted):
		status, code, title, detail = http.StatusConflict, "trip_completed", "Trip completed", "Operation is not allowed for a completed trip"
	case errors.Is(err, trip.ErrDriverBusy):
		status, code, title, detail = http.StatusConflict, "driver_busy", "Driver busy", "Driver already has an active trip"
	}
	return status, api.Problem{Type: "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"), Title: title, Status: int32(status), Code: code, Detail: &detail}
}

func Write(w http.ResponseWriter, r *http.Request, err error) {
	status, body := Problem(err)
	response.SetError(r.Context(), err)
	response.SetErrorType(r.Context(), body.Code)
	instance := r.URL.Path
	body.Instance = &instance
	w.Header().Del("Location")
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", "application/problem+json")
	_ = response.Encode(w, status, body)
}

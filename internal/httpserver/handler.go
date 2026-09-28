package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/trip"
)

type Handler struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
	service      *trip.Service
}

var _ api.ServerInterface = (*Handler)(nil)

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	input, err := decodeCreate(r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", "Request body is invalid")
		return
	}
	created, err := h.service.Create(r.Context(), trip.CreateInput{
		UserID: input.UserId, DriverID: input.DriverId, Price: input.Price,
		StartPoint: trip.Coordinates{Latitude: input.StartPoint.Latitude, Longitude: input.StartPoint.Longitude},
		EndPoint:   trip.Coordinates{Latitude: input.EndPoint.Latitude, Longitude: input.EndPoint.Longitude},
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+created.ID.String())
	writeJSON(w, http.StatusCreated, responseTrip(created))
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	found, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, responseTrip(found))
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(body) != 0 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", "Finish request must have an empty body")
		return
	}
	completed, err := h.service.Finish(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, responseTrip(completed))
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func decodeCreate(r *http.Request) (api.CreateTripJSONRequestBody, error) {
	var input api.CreateTripJSONRequestBody
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return input, errors.New("expected application/json")
	}
	decoder := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return input, err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return input, errors.New("expected a single JSON object")
	}
	if err := requiredFields(raw, "user_id", "driver_id", "start_point", "end_point", "price"); err != nil {
		return input, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return input, err
	}
	for _, name := range []string{"start_point", "end_point"} {
		if err := requiredFields(fields[name], "latitude", "longitude"); err != nil {
			return input, err
		}
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	if input.UserId == uuid.Nil || input.DriverId == uuid.Nil || input.Price < 0 || !validCoordinates(input.StartPoint) || !validCoordinates(input.EndPoint) {
		return input, errors.New("invalid trip fields")
	}
	return input, nil
}

func requiredFields(raw json.RawMessage, names ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if len(fields) != len(names) {
		return errors.New("missing or unknown field")
	}
	for _, name := range names {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("required field is missing or null")
		}
	}
	return nil
}

func validCoordinates(point api.Coordinates) bool {
	return point.Latitude >= -90 && point.Latitude <= 90 && point.Longitude >= -180 && point.Longitude <= 180
}

func responseTrip(value trip.Trip) api.Trip {
	return api.Trip{
		Id: value.ID, UserId: value.UserID, DriverId: value.DriverID,
		StartPoint: api.Coordinates{Latitude: value.StartPoint.Latitude, Longitude: value.StartPoint.Longitude},
		EndPoint:   api.Coordinates{Latitude: value.EndPoint.Latitude, Longitude: value.EndPoint.Longitude},
		Price:      value.Price, Status: api.TripStatus(value.Status), StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
	}
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trip.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found", "Trip was not found")
	case errors.Is(err, trip.ErrCompleted):
		writeProblem(w, r, http.StatusConflict, "trip_completed", "Trip completed", "Operation is not allowed for a completed trip")
	case errors.Is(err, trip.ErrDriverBusy):
		writeProblem(w, r, http.StatusConflict, "driver_busy", "Driver busy", "Driver already has an active trip")
	default:
		slog.Error("trip request failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
	}
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	instance := r.URL.Path
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(api.Problem{
		Type:  "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"),
		Title: title, Status: int32(status), Code: code, Detail: &detail, Instance: &instance,
	}); err != nil {
		slog.Error("write problem response", "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write JSON response", "error", err)
	}
}

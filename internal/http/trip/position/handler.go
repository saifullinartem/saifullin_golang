package position

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/respond"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

func (h *Handler) create(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		domainerror.Write(w, r, &trip.ValidationError{Field: "Content-Type", Reason: "must be application/json", Cause: err})
		return
	}
	var body struct {
		Coordinates struct {
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		} `json:"coordinates"`
		RecordedAt time.Time `json:"recorded_at"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		domainerror.Write(w, r, &trip.ValidationError{Field: "body", Reason: "must be valid JSON", Cause: err})
		return
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		domainerror.Write(w, r, &trip.ValidationError{Field: "body", Reason: "must contain a single JSON object", Cause: err})
		return
	}
	if body.Coordinates.Latitude == nil || body.Coordinates.Longitude == nil {
		domainerror.Write(w, r, &trip.ValidationError{Field: "coordinates", Reason: "latitude and longitude are required"})
		return
	}
	value, err := h.usecase.Save(r.Context(), model.PositionInput{TripID: id, Coordinates: model.Coordinates{Latitude: *body.Coordinates.Latitude, Longitude: *body.Coordinates.Longitude}, RecordedAt: body.RecordedAt})
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	if h.cancel != nil {
		h.cancel(id)
	}
	respond.JSON(w, r, http.StatusCreated, api.TripPosition{Id: value.ID, TripId: value.TripID, Coordinates: api.Coordinates{Latitude: value.Coordinates.Latitude, Longitude: value.Coordinates.Longitude}, RecordedAt: value.RecordedAt})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	values, err := h.usecase.List(r.Context(), id)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	result := api.TripPositionsResponse{Positions: make([]api.TripPosition, 0, len(values))}
	for _, value := range values {
		result.Positions = append(result.Positions, api.TripPosition{Id: value.ID, TripId: value.TripID, Coordinates: api.Coordinates{Latitude: value.Coordinates.Latitude, Longitude: value.Coordinates.Longitude}, RecordedAt: value.RecordedAt})
	}
	respond.JSON(w, r, http.StatusOK, result)
}

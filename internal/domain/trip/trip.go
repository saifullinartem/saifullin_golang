package trip

import (
	"math"
	"time"

	"github.com/google/uuid"
)

const (
	Active    = "active"
	Completed = "completed"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != Active {
		return ErrTripCompleted
	}
	t.Status = Completed
	t.FinishedAt = &at
	return nil
}

type CreateInput struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
}

func (i CreateInput) Validate() error {
	if i.UserID == uuid.Nil {
		return &ValidationError{Field: "user_id", Reason: "must be a non-zero UUID"}
	}
	if i.DriverID == uuid.Nil {
		return &ValidationError{Field: "driver_id", Reason: "must be a non-zero UUID"}
	}
	if i.Price < 0 {
		return &ValidationError{Field: "price", Reason: "must be non-negative"}
	}
	for _, p := range []struct {
		name  string
		point Coordinates
	}{{"start_point", i.StartPoint}, {"end_point", i.EndPoint}} {
		if math.IsNaN(p.point.Latitude) || p.point.Latitude < -90 || p.point.Latitude > 90 {
			return &ValidationError{Field: p.name + ".latitude", Reason: "must be between -90 and 90"}
		}
		if math.IsNaN(p.point.Longitude) || p.point.Longitude < -180 || p.point.Longitude > 180 {
			return &ValidationError{Field: p.name + ".longitude", Reason: "must be between -180 and 180"}
		}
	}
	return nil
}

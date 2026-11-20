package position

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type (
	savedMetric interface{ Saved() }
	Usecase     struct {
		repository repository
		tx         txManager
		now        func() time.Time
		metrics    savedMetric
	}
)

func New(r repository, tx txManager, metrics ...savedMetric) *Usecase {
	u := &Usecase{repository: r, tx: tx, now: time.Now}
	if len(metrics) > 0 {
		u.metrics = metrics[0]
	}
	return u
}

func (u *Usecase) Save(ctx context.Context, input model.PositionInput) (model.Position, error) {
	if input.TripID == uuid.Nil {
		return model.Position{}, &trip.ValidationError{Field: "trip_id", Reason: "must be a non-zero UUID"}
	}
	if math.IsNaN(input.Coordinates.Latitude) || input.Coordinates.Latitude < -90 || input.Coordinates.Latitude > 90 {
		return model.Position{}, &trip.ValidationError{Field: "coordinates.latitude", Reason: "must be between -90 and 90"}
	}
	if math.IsNaN(input.Coordinates.Longitude) || input.Coordinates.Longitude < -180 || input.Coordinates.Longitude > 180 {
		return model.Position{}, &trip.ValidationError{Field: "coordinates.longitude", Reason: "must be between -180 and 180"}
	}
	now := u.now()
	if input.RecordedAt.IsZero() || input.RecordedAt.After(now.Add(time.Minute)) {
		return model.Position{}, &trip.ValidationError{Field: "recorded_at", Reason: "must be a valid time no more than one minute in the future"}
	}
	var result model.Position
	err := u.tx.Do(ctx, func(txctx context.Context) error {
		var err error
		result, err = u.repository.SavePosition(txctx, input)
		return err
	})
	if err != nil {
		return model.Position{}, fmt.Errorf("save position: %w", err)
	}
	if u.metrics != nil {
		u.metrics.Saved()
	}
	return result, nil
}

func (u *Usecase) List(ctx context.Context, id uuid.UUID) ([]model.Position, error) {
	if id == uuid.Nil {
		return nil, &trip.ValidationError{Field: "tripId", Reason: "must be a valid UUID"}
	}
	positions, err := u.repository.ListPositions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	return positions, nil
}

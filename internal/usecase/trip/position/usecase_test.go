package position

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

func TestSaveRejectsInvalidPosition(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	u := &Usecase{now: func() time.Time { return now }}
	for _, tc := range []struct {
		field string
		input model.PositionInput
	}{
		{"trip_id", model.PositionInput{}},
		{"coordinates.latitude", model.PositionInput{TripID: uuid.New(), Coordinates: model.Coordinates{Latitude: 91}}},
		{"coordinates.longitude", model.PositionInput{TripID: uuid.New(), Coordinates: model.Coordinates{Longitude: -181}}},
		{"recorded_at", model.PositionInput{TripID: uuid.New(), RecordedAt: now.Add(time.Minute + time.Second)}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			_, err := u.Save(t.Context(), tc.input)
			var invalid *trip.ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tc.field {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

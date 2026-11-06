package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
)

type (
	Trip         = trip.Trip
	Coordinates  = trip.Coordinates
	CreateInput  = trip.CreateInput
	StatusChange struct {
		TripID     uuid.UUID
		FromStatus *string
		ToStatus   string
		Reason     *string
		ChangedAt  time.Time
	}
)

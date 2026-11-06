package trip

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type TxManager interface {
	Do(context.Context, func(context.Context) error) error
}

var (
	ErrNotFound   = errors.New("trip not found")
	ErrCompleted  = errors.New("trip already completed")
	ErrDriverBusy = errors.New("driver already has an active trip")
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

type StatusChange struct {
	TripID     uuid.UUID
	FromStatus *string
	ToStatus   string
	Reason     *string
	ChangedAt  time.Time
}

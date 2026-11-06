package create

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type Usecase struct {
	repository tripRepository
	history    historyRepository
	tx         txManager
	clock      clock
	events     events
	logger     *slog.Logger
}

func New(r tripRepository, h historyRepository, tx txManager, c clock, e events, l *slog.Logger) *Usecase {
	return &Usecase{r, h, tx, c, e, l}
}

func (s *Usecase) Create(ctx context.Context, input model.CreateInput) (model.Trip, error) {
	if err := input.Validate(); err != nil {
		return model.Trip{}, fmt.Errorf("validate trip: %w", err)
	}
	created := model.Trip{ID: uuid.New(), UserID: input.UserID, DriverID: input.DriverID, StartPoint: input.StartPoint, EndPoint: input.EndPoint, Price: input.Price, Status: trip.Active, StartedAt: s.clock.Now().UTC().Truncate(time.Microsecond)}
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repository.Create(ctx, created); err != nil {
			return fmt.Errorf("save trip: %w", err)
		}
		if err := s.history.Add(ctx, model.StatusChange{TripID: created.ID, ToStatus: trip.Active, ChangedAt: created.StartedAt}); err != nil {
			return fmt.Errorf("save creation history: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("create trip: %w", err)
	}
	s.events.Created(ctx)
	s.logger.InfoContext(ctx, "trip created", "trip_id", created.ID)
	return created, nil
}

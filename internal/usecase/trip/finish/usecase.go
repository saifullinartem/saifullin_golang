package finish

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

func (s *Usecase) Finish(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	var completed model.Trip
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		found, err := s.repository.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("fetch trip: %w", err)
		}
		if err := found.Finish(s.clock.Now().UTC().Truncate(time.Microsecond)); err != nil {
			return fmt.Errorf("finish trip state: %w", err)
		}
		completed, err = s.repository.Finish(ctx, id, *found.FinishedAt)
		if err != nil {
			return fmt.Errorf("save completed trip: %w", err)
		}
		from := trip.Active
		if err := s.history.Add(ctx, model.StatusChange{TripID: id, FromStatus: &from, ToStatus: trip.Completed, ChangedAt: *completed.FinishedAt}); err != nil {
			return fmt.Errorf("save completion history: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	s.events.Completed(ctx)
	s.logger.InfoContext(ctx, "trip completed", "trip_id", id)
	return completed, nil
}

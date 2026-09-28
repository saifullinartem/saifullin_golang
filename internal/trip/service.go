package trip

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type CreateInput struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
}

type Service struct {
	repository *Repository
	history    *HistoryRepository
	tx         TxManager
}

func NewService(repository *Repository, history *HistoryRepository, tx TxManager) *Service {
	return &Service{repository: repository, history: history, tx: tx}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Trip, error) {
	created := Trip{
		ID: uuid.New(), UserID: input.UserID, DriverID: input.DriverID,
		StartPoint: input.StartPoint, EndPoint: input.EndPoint, Price: input.Price,
		Status: Active, StartedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repository.Create(ctx, created); err != nil {
			return err
		}
		return s.history.Add(ctx, StatusChange{TripID: created.ID, ToStatus: Active, ChangedAt: created.StartedAt})
	})
	if err != nil {
		return Trip{}, err
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Trip, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) Finish(ctx context.Context, id uuid.UUID) (Trip, error) {
	var completed Trip
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		completed, err = s.repository.Finish(ctx, id, time.Now().UTC().Truncate(time.Microsecond))
		if err != nil {
			return err
		}
		from := Active
		return s.history.Add(ctx, StatusChange{TripID: id, FromStatus: &from, ToStatus: Completed, ChangedAt: *completed.FinishedAt})
	})
	if err != nil {
		return Trip{}, err
	}
	return completed, nil
}

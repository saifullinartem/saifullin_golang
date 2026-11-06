package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type Usecase struct{ repository tripRepository }

func New(r tripRepository) *Usecase { return &Usecase{r} }
func (s *Usecase) Get(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return found, nil
}

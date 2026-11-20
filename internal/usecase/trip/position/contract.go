package position

import (
	"context"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type repository interface {
	SavePosition(context.Context, model.PositionInput) (model.Position, error)
	ListPositions(context.Context, uuid.UUID) ([]model.Position, error)
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

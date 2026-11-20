//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type tripRepository interface {
	Get(context.Context, uuid.UUID) (model.Trip, error)
	Finish(context.Context, uuid.UUID, time.Time) (model.Trip, error)
}
type historyRepository interface {
	Add(context.Context, model.StatusChange) error
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}
type (
	events interface{ Completed(context.Context) }
	clock  interface{ Now() time.Time }
)

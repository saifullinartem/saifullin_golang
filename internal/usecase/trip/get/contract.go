//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package get_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type tripRepository interface {
	Get(context.Context, uuid.UUID) (model.Trip, error)
}

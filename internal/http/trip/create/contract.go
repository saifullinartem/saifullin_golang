//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type usecase interface {
	Create(context.Context, model.CreateInput) (model.Trip, error)
}

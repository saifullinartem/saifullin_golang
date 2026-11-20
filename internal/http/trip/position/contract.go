package position

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type usecase interface {
	Save(context.Context, model.PositionInput) (model.Position, error)
	List(context.Context, uuid.UUID) ([]model.Position, error)
}
type Handler struct {
	usecase usecase
	cancel  func(uuid.UUID)
}

func New(u usecase, cancel ...func(uuid.UUID)) *Handler {
	h := &Handler{usecase: u}
	if len(cancel) > 0 {
		h.cancel = cancel[0]
	}
	return h
}

func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	h.create(w, r, id)
}

func (h *Handler) ListTripPositions(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	h.list(w, r, id)
}

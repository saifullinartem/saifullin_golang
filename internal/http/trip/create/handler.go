package create

import (
	"net/http"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/respond"
	"github.com/saifullinartem/saifullin_golang/internal/http/trip/convert"
)

type Handler struct{ usecase usecase }

func New(u usecase) *Handler { return &Handler{u} }
func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	input, err := decodeCreate(r)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	created, err := h.usecase.Create(r.Context(), toInput(input))
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+created.ID.String())
	respond.JSON(w, r, http.StatusCreated, convert.Trip(created))
}

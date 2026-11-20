package get

import (
	"net/http"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/respond"
	"github.com/saifullinartem/saifullin_golang/internal/http/trip/convert"
)

type Handler struct{ usecase usecase }

func New(u usecase) *Handler { return &Handler{u} }
func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	found, err := h.usecase.Get(r.Context(), id)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	respond.JSON(w, r, http.StatusOK, convert.Trip(found))
}

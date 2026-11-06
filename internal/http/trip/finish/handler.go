package finish

import (
	"io"
	"net/http"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/respond"
	"github.com/saifullinartem/saifullin_golang/internal/http/trip/convert"
)

type Handler struct{ usecase usecase }

func New(u usecase) *Handler { return &Handler{u} }
func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil {
		domainerror.Write(w, r, &trip.ValidationError{Field: "body", Reason: "could not read request body", Cause: err})
		return
	}
	if len(body) != 0 {
		domainerror.Write(w, r, &trip.ValidationError{Field: "body", Reason: "Finish request must have an empty body"})
		return
	}
	completed, err := h.usecase.Finish(r.Context(), id)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	respond.JSON(w, r, http.StatusOK, convert.Trip(completed))
}

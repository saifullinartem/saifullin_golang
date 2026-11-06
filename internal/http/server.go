package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/respond"
	"github.com/saifullinartem/saifullin_golang/internal/http/response"
)

type Handler struct {
	createHandler
	getHandler
	finishHandler
	db      pinger
	timeout time.Duration
}

func New(c createHandler, g getHandler, f finishHandler, p pinger, timeout time.Duration) *Handler {
	return &Handler{c, g, f, p, timeout}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		response.SetError(r.Context(), err)
		response.SetErrorType(r.Context(), "database_unavailable")
		respond.JSON(w, r, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	respond.JSON(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func ParameterError(w http.ResponseWriter, r *http.Request, err error) {
	field, reason := "parameters", "Request parameters are invalid"
	var format *api.InvalidParamFormatError
	var many *api.TooManyValuesForParamError
	switch {
	case errors.As(err, &format):
		field, reason = format.ParamName, "must be a valid UUID"
	case errors.As(err, &many):
		field, reason = many.ParamName, "must have a single value"
	}
	domainerror.Write(w, r, &trip.ValidationError{Field: field, Reason: reason, Cause: err})
}

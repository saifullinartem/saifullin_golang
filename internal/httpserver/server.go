package httpserver

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/config"
	"github.com/saifullinartem/saifullin_golang/internal/trip"
)

func New(cfg config.Config, pool *pgxpool.Pool, queryTimeout time.Duration, service *trip.Service) *http.Server {
	handler := &Handler{pool: pool, queryTimeout: queryTimeout, service: service}
	router := api.HandlerWithOptions(handler, api.ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", "Request parameters are invalid")
		},
	})

	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}
}

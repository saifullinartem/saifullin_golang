package http

import (
	"context"
	"net/http"

	"github.com/saifullinartem/saifullin_golang/api"
)

//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package http_test
type pinger interface{ Ping(context.Context) error }

type createHandler interface {
	CreateTrip(http.ResponseWriter, *http.Request, api.CreateTripParams)
}
type getHandler interface {
	GetTrip(http.ResponseWriter, *http.Request, api.TripId)
}
type finishHandler interface {
	FinishTrip(http.ResponseWriter, *http.Request, api.TripId)
}

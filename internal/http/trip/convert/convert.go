package convert

import (
	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

func Trip(value model.Trip) api.Trip {
	return api.Trip{
		Id: value.ID, UserId: value.UserID, DriverId: value.DriverID,
		StartPoint: api.Coordinates{Latitude: value.StartPoint.Latitude, Longitude: value.StartPoint.Longitude},
		EndPoint:   api.Coordinates{Latitude: value.EndPoint.Latitude, Longitude: value.EndPoint.Longitude},
		Price:      value.Price, Status: api.TripStatus(value.Status), StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
	}
}

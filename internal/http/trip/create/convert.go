package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

const bodyField = "body"

func decodeCreate(r *http.Request) (api.CreateTripJSONRequestBody, error) {
	var input api.CreateTripJSONRequestBody
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return input, &trip.ValidationError{Field: "Content-Type", Reason: "must be application/json", Cause: err}
	}
	decoder := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return input, bodyError(err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return input, bodyError(err)
		}
		return input, &trip.ValidationError{Field: bodyField, Reason: "must contain a single JSON object"}
	}
	fields, err := objectFields(raw, "", "user_id", "driver_id", "start_point", "end_point", "price")
	if err != nil {
		return input, err
	}
	for _, field := range []struct {
		name, reason string
		target       any
	}{
		{"user_id", "must be a valid UUID", &input.UserId},
		{"driver_id", "must be a valid UUID", &input.DriverId},
		{"price", "must be an integer", &input.Price},
	} {
		if err := decodeField(fields[field.name], field.name, field.reason, field.target); err != nil {
			return input, err
		}
	}
	if input.StartPoint, err = decodeCoordinates(fields["start_point"], "start_point"); err != nil {
		return input, err
	}
	if input.EndPoint, err = decodeCoordinates(fields["end_point"], "end_point"); err != nil {
		return input, err
	}
	if err := toInput(input).Validate(); err != nil {
		return input, err
	}
	return input, nil
}

func bodyError(err error) *trip.ValidationError {
	reason := "could not read JSON body"
	var syntax *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		reason = "request body is required"
	case errors.Is(err, io.ErrUnexpectedEOF):
		reason = "incomplete JSON body"
	case errors.As(err, &syntax):
		reason = fmt.Sprintf("invalid JSON syntax at byte %d", syntax.Offset)
	}
	return &trip.ValidationError{Field: bodyField, Reason: reason, Cause: err}
}

func objectFields(raw json.RawMessage, prefix string, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, &trip.ValidationError{Field: fieldPath(prefix, ""), Reason: "must be a JSON object", Cause: err}
	}
	if fields == nil {
		return nil, &trip.ValidationError{Field: fieldPath(prefix, ""), Reason: "must be a JSON object"}
	}
	for _, name := range names {
		value, exists := fields[name]
		if !exists {
			return nil, &trip.ValidationError{Field: fieldPath(prefix, name), Reason: "is required"}
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, &trip.ValidationError{Field: fieldPath(prefix, name), Reason: "must not be null"}
		}
	}
	unknown := make([]string, 0)
	for name := range fields {
		known := false
		for _, expected := range names {
			if name == expected {
				known = true
				break
			}
		}
		if !known {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		return nil, &trip.ValidationError{Field: fieldPath(prefix, unknown[0]), Reason: "unknown field"}
	}
	return fields, nil
}

func fieldPath(prefix, name string) string {
	if prefix == "" {
		if name == "" {
			return bodyField
		}
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}

func decodeField(raw json.RawMessage, name, reason string, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return &trip.ValidationError{Field: name, Reason: reason, Cause: err}
	}
	return nil
}

func decodeCoordinates(raw json.RawMessage, name string) (api.Coordinates, error) {
	var point api.Coordinates
	fields, err := objectFields(raw, name, "latitude", "longitude")
	if err != nil {
		return point, err
	}
	if err := decodeField(fields["latitude"], name+".latitude", "must be a number", &point.Latitude); err != nil {
		return point, err
	}
	if err := decodeField(fields["longitude"], name+".longitude", "must be a number", &point.Longitude); err != nil {
		return point, err
	}
	return point, nil
}

func toInput(input api.CreateTripJSONRequestBody) model.CreateInput {
	return model.CreateInput{UserID: input.UserId, DriverID: input.DriverId, Price: input.Price, StartPoint: model.Coordinates{Latitude: input.StartPoint.Latitude, Longitude: input.StartPoint.Longitude}, EndPoint: model.Coordinates{Latitude: input.EndPoint.Latitude, Longitude: input.EndPoint.Longitude}}
}

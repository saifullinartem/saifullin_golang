package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oapi-codegen/runtime"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const (
	Ok          HealthResponseStatus = "ok"
	Unavailable HealthResponseStatus = "unavailable"
)

func (e HealthResponseStatus) Valid() bool {
	switch e {
	case Ok:
		return true
	case Unavailable:
		return true
	default:
		return false
	}
}

const (
	Active    TripStatus = "active"
	Completed TripStatus = "completed"
)

func (e TripStatus) Valid() bool {
	switch e {
	case Active:
		return true
	case Completed:
		return true
	default:
		return false
	}
}

type Coordinates struct {
	Latitude float64 `json:"latitude"`

	Longitude float64 `json:"longitude"`
}

type HealthResponse struct {
	Status HealthResponseStatus `json:"status"`
}

type HealthResponseStatus string

type Problem struct {
	Code     string  `json:"code"`
	Detail   *string `json:"detail,omitempty"`
	Instance *string `json:"instance,omitempty"`
	Status   int32   `json:"status"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
}

type Trip struct {
	DriverId openapi_types.UUID `json:"driver_id"`

	EndPoint       Coordinates        `json:"end_point"`
	FinishedAt     *time.Time         `json:"finished_at,omitempty"`
	Id             openapi_types.UUID `json:"id"`
	LastPositionAt *time.Time         `json:"last_position_at,omitempty"`

	Price int64 `json:"price"`

	StartPoint Coordinates        `json:"start_point"`
	StartedAt  time.Time          `json:"started_at"`
	Status     TripStatus         `json:"status"`
	UserId     openapi_types.UUID `json:"user_id"`
}

type TripData struct {
	DriverId openapi_types.UUID `json:"driver_id"`

	EndPoint Coordinates `json:"end_point"`

	Price int64 `json:"price"`

	StartPoint Coordinates        `json:"start_point"`
	UserId     openapi_types.UUID `json:"user_id"`
}

type TripStatus string

type IdempotencyKey = openapi_types.UUID

type TripId = openapi_types.UUID

type BadRequest = Problem

type CreateTripConflict = Problem

type InternalError = Problem

type TripCompleted = Problem

type TripNotFound = Problem

type CreateTripParams struct {
	IdempotencyKey *IdempotencyKey `json:"Idempotency-Key,omitempty"`
}

type CreateTripJSONRequestBody = TripData

type ServerInterface interface {
	CreateTrip(w http.ResponseWriter, r *http.Request, params CreateTripParams)
	GetTrip(w http.ResponseWriter, r *http.Request, tripId TripId)
	FinishTrip(w http.ResponseWriter, r *http.Request, tripId TripId)
	Health(w http.ResponseWriter, r *http.Request)
	Ready(w http.ResponseWriter, r *http.Request)
}

type Unimplemented struct{}

func (_ Unimplemented) CreateTrip(w http.ResponseWriter, r *http.Request, params CreateTripParams) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (_ Unimplemented) GetTrip(w http.ResponseWriter, r *http.Request, tripId TripId) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (_ Unimplemented) FinishTrip(w http.ResponseWriter, r *http.Request, tripId TripId) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (_ Unimplemented) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (_ Unimplemented) Ready(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

type ServerInterfaceWrapper struct {
	Handler            ServerInterface
	HandlerMiddlewares []MiddlewareFunc
	ErrorHandlerFunc   func(w http.ResponseWriter, r *http.Request, err error)
}

type MiddlewareFunc func(http.Handler) http.Handler

func (siw *ServerInterfaceWrapper) CreateTrip(w http.ResponseWriter, r *http.Request) {

	var err error
	_ = err

	var params CreateTripParams

	headers := r.Header

	if valueList, found := headers[http.CanonicalHeaderKey("Idempotency-Key")]; found {
		var IdempotencyKey IdempotencyKey
		n := len(valueList)
		if n != 1 {
			siw.ErrorHandlerFunc(w, r, &TooManyValuesForParamError{ParamName: "Idempotency-Key", Count: n})
			return
		}

		err = runtime.BindStyledParameterWithOptions("simple", "Idempotency-Key", valueList[0], &IdempotencyKey, runtime.BindStyledParameterOptions{ParamLocation: runtime.ParamLocationHeader, Explode: false, Required: false, Type: "string", Format: "uuid"})
		if err != nil {
			siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "Idempotency-Key", Err: err})
			return
		}

		params.IdempotencyKey = &IdempotencyKey

	}

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.CreateTrip(w, r, params)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

func (siw *ServerInterfaceWrapper) GetTrip(w http.ResponseWriter, r *http.Request) {

	var err error
	_ = err

	var tripId TripId

	err = runtime.BindStyledParameterWithOptions("simple", "tripId", chi.URLParam(r, "tripId"), &tripId, runtime.BindStyledParameterOptions{ParamLocation: runtime.ParamLocationPath, Explode: false, Required: true, Type: "string", Format: "uuid", ValueIsUnescaped: r.URL.RawPath == ""})
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "tripId", Err: err})
		return
	}

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.GetTrip(w, r, tripId)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

func (siw *ServerInterfaceWrapper) FinishTrip(w http.ResponseWriter, r *http.Request) {

	var err error
	_ = err

	var tripId TripId

	err = runtime.BindStyledParameterWithOptions("simple", "tripId", chi.URLParam(r, "tripId"), &tripId, runtime.BindStyledParameterOptions{ParamLocation: runtime.ParamLocationPath, Explode: false, Required: true, Type: "string", Format: "uuid", ValueIsUnescaped: r.URL.RawPath == ""})
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "tripId", Err: err})
		return
	}

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.FinishTrip(w, r, tripId)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

func (siw *ServerInterfaceWrapper) Health(w http.ResponseWriter, r *http.Request) {

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.Health(w, r)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

func (siw *ServerInterfaceWrapper) Ready(w http.ResponseWriter, r *http.Request) {

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.Ready(w, r)
	}))

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r)
}

type UnescapedCookieParamError struct {
	ParamName string
	Err       error
}

func (e *UnescapedCookieParamError) Error() string {
	return fmt.Sprintf("error unescaping cookie parameter '%s'", e.ParamName)
}

func (e *UnescapedCookieParamError) Unwrap() error {
	return e.Err
}

type UnmarshalingParamError struct {
	ParamName string
	Err       error
}

func (e *UnmarshalingParamError) Error() string {
	return fmt.Sprintf("Error unmarshaling parameter %s as JSON: %s", e.ParamName, e.Err.Error())
}

func (e *UnmarshalingParamError) Unwrap() error {
	return e.Err
}

type RequiredParamError struct {
	ParamName string
}

func (e *RequiredParamError) Error() string {
	return fmt.Sprintf("Query argument %s is required, but not found", e.ParamName)
}

type RequiredHeaderError struct {
	ParamName string
	Err       error
}

func (e *RequiredHeaderError) Error() string {
	return fmt.Sprintf("Header parameter %s is required, but not found", e.ParamName)
}

func (e *RequiredHeaderError) Unwrap() error {
	return e.Err
}

type InvalidParamFormatError struct {
	ParamName string
	Err       error
}

func (e *InvalidParamFormatError) Error() string {
	return fmt.Sprintf("Invalid format for parameter %s: %s", e.ParamName, e.Err.Error())
}

func (e *InvalidParamFormatError) Unwrap() error {
	return e.Err
}

type TooManyValuesForParamError struct {
	ParamName string
	Count     int
}

func (e *TooManyValuesForParamError) Error() string {
	return fmt.Sprintf("Expected one value for %s, got %d", e.ParamName, e.Count)
}

func Handler(si ServerInterface) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{})
}

type ChiServerOptions struct {
	BaseURL          string
	BaseRouter       chi.Router
	Middlewares      []MiddlewareFunc
	ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)
}

func HandlerFromMux(si ServerInterface, r chi.Router) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{
		BaseRouter: r,
	})
}

func HandlerFromMuxWithBaseURL(si ServerInterface, r chi.Router, baseURL string) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{
		BaseURL:    baseURL,
		BaseRouter: r,
	})
}

func HandlerWithOptions(si ServerInterface, options ChiServerOptions) http.Handler {
	r := options.BaseRouter

	if r == nil {
		r = chi.NewRouter()
	}
	if options.ErrorHandlerFunc == nil {
		options.ErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
	wrapper := ServerInterfaceWrapper{
		Handler:            si,
		HandlerMiddlewares: options.Middlewares,
		ErrorHandlerFunc:   options.ErrorHandlerFunc,
	}

	r.Group(func(r chi.Router) {
		r.Post(options.BaseURL+"/api/v1/trips", wrapper.CreateTrip)
	})
	r.Group(func(r chi.Router) {
		r.Get(options.BaseURL+"/api/v1/trips/{tripId}", wrapper.GetTrip)
	})
	r.Group(func(r chi.Router) {
		r.Post(options.BaseURL+"/api/v1/trips/{tripId}/finish", wrapper.FinishTrip)
	})
	r.Group(func(r chi.Router) {
		r.Get(options.BaseURL+"/health", wrapper.Health)
	})
	r.Group(func(r chi.Router) {
		r.Get(options.BaseURL+"/ready", wrapper.Ready)
	})

	return r
}

package trip

import "errors"

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)

type ValidationError struct {
	Field  string
	Reason string
	Cause  error
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Reason }

func (e *ValidationError) Unwrap() error { return e.Cause }

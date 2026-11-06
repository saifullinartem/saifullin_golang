package response

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type State struct {
	Err       error
	ErrorType string
	Stack     string
	Recover   func(http.ResponseWriter, *http.Request, http.Handler)
}
type stateKey struct{}

func WithState(ctx context.Context, s *State) context.Context {
	return context.WithValue(ctx, stateKey{}, s)
}

func SetError(ctx context.Context, err error) {
	if s, ok := ctx.Value(stateKey{}).(*State); ok {
		s.Err = err
	}
}

func JSON(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	return Encode(w, status, value)
}

func Encode(w http.ResponseWriter, status int, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
	return nil
}

func Serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if state, ok := r.Context().Value(stateKey{}).(*State); ok && state.Recover != nil {
		state.Recover(w, r, next)
		return
	}
	next.ServeHTTP(w, r)
}

func SetPanic(ctx context.Context, err error, stack string) {
	if state, ok := ctx.Value(stateKey{}).(*State); ok {
		state.Err = err
		state.Stack = stack
		state.ErrorType = "panic"
	}
}

func Error(ctx context.Context) error {
	if state, ok := ctx.Value(stateKey{}).(*State); ok {
		return state.Err
	}
	return nil
}

func Stack(ctx context.Context) string {
	if state, ok := ctx.Value(stateKey{}).(*State); ok {
		return state.Stack
	}
	return ""
}

func SetErrorType(ctx context.Context, value string) {
	if state, ok := ctx.Value(stateKey{}).(*State); ok {
		state.ErrorType = value
	}
}

func ErrorType(ctx context.Context) string {
	if state, ok := ctx.Value(stateKey{}).(*State); ok && state.ErrorType != "" {
		return state.ErrorType
	}
	return "internal_error"
}

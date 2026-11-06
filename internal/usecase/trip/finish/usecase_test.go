package finish_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/model"
	"github.com/saifullinartem/saifullin_golang/internal/usecase/trip/finish"
)

func TestFinish(t *testing.T) {
	failed := errors.New("database unavailable")
	for _, tt := range []struct {
		name, status                                            string
		readErr, saveErr, historyErr, txErr, commitErr, wantErr error
	}{
		{name: "success", status: trip.Active},
		{name: "completed", status: trip.Completed, wantErr: trip.ErrTripCompleted},
		{name: "not found", readErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "read failure", readErr: failed, wantErr: failed},
		{name: "concurrent finish", status: trip.Active, saveErr: trip.ErrTripCompleted, wantErr: trip.ErrTripCompleted},
		{name: "save failure", status: trip.Active, saveErr: failed, wantErr: failed},
		{name: "history failure", status: trip.Active, historyErr: failed, wantErr: failed},
		{name: "transaction failure", txErr: failed, wantErr: failed},
		{name: "commit failure", status: trip.Active, commitErr: failed, wantErr: failed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMockhistoryRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			clock := NewMockclock(ctrl)
			events := NewMockevents(ctrl)
			ctx := context.Background()
			id := uuid.New()
			rawAt := time.Date(2026, 9, 1, 12, 0, 0, 123456789, time.FixedZone("UTC+3", 3*60*60))
			at := rawAt.UTC().Truncate(time.Microsecond)
			value := model.Trip{ID: id, Status: tt.status}
			tx.EXPECT().Do(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				if tt.txErr != nil {
					return tt.txErr
				}
				if err := fn(ctx); err != nil {
					return err
				}
				return tt.commitErr
			})
			if tt.txErr == nil {
				repo.EXPECT().Get(ctx, id).Return(value, tt.readErr)
				if tt.readErr == nil {
					clock.EXPECT().Now().Return(rawAt)
					if tt.status == trip.Active {
						value.Status = trip.Completed
						value.FinishedAt = &at
						repo.EXPECT().Finish(ctx, id, at).Return(value, tt.saveErr)
						if tt.saveErr == nil {
							from := trip.Active
							history.EXPECT().Add(ctx, model.StatusChange{TripID: id, FromStatus: &from, ToStatus: trip.Completed, ChangedAt: at}).Return(tt.historyErr)
						}
					}
				}
			}
			if tt.wantErr == nil {
				events.EXPECT().Completed(ctx)
			}
			var logs bytes.Buffer
			u := finish.New(repo, history, tx, clock, events, slog.New(slog.NewJSONHandler(&logs, nil)))
			got, err := u.Finish(ctx, id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v,want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				assertBusinessLog(t, logs.Bytes(), "trip completed", got.ID)
			} else if logs.Len() != 0 {
				t.Fatal("failed transaction emitted a business log")
			}
			if tt.wantErr != nil {
				if !reflect.DeepEqual(got, model.Trip{}) {
					t.Fatal("failure returned trip")
				}
				return
			}
			if !reflect.DeepEqual(got, value) {
				t.Fatalf("trip=%+v,want %+v", got, value)
			}
		})
	}
}

func assertBusinessLog(t *testing.T, data []byte, message string, id uuid.UUID) {
	t.Helper()
	var event struct {
		Message string `json:"msg"`
		Level   string `json:"level"`
		TripID  string `json:"trip_id"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("business log: %v", err)
	}
	if event.Message != message || event.Level != "INFO" || event.TripID != id.String() {
		t.Fatalf("business log=%s", data)
	}
}

package get_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/model"
	"github.com/saifullinartem/saifullin_golang/internal/usecase/trip/get"
)

func TestGet(t *testing.T) {
	for _, wantErr := range []error{nil, trip.ErrTripNotFound, errors.New("repository failed")} {
		t.Run(errorName(wantErr), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			ctx := context.Background()
			id := uuid.New()
			want := model.Trip{ID: id, Status: trip.Active}
			if wantErr != nil {
				want = model.Trip{}
			}
			repo.EXPECT().Get(ctx, id).Return(want, wantErr)
			got, err := get.New(repo).Get(ctx, id)
			if !errors.Is(err, wantErr) || !reflect.DeepEqual(got, want) {
				t.Fatalf("Get=(%+v,%v), want (%+v,%v)", got, err, want, wantErr)
			}
		})
	}
}

func errorName(err error) string {
	if err == nil {
		return "success"
	}
	return err.Error()
}

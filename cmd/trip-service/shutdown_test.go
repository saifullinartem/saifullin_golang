package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestCloseWithinContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	app := NewMockcloser(ctrl)
	ctx := context.Background()
	app.EXPECT().Close(ctx)
	if err := closeWithinContext(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestCloseDoesNotExceedBudget(t *testing.T) {
	ctrl := gomock.NewController(t)
	app := NewMockcloser(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	app.EXPECT().Close(ctx).Do(func(context.Context) {
		close(started)
		<-release
		close(finished)
	})
	t.Cleanup(func() { close(release); <-finished })
	result := make(chan error, 1)
	go func() { result <- closeWithinContext(ctx, app) }()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cleanup error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup ignored cancellation while resource remained blocked")
	}
}

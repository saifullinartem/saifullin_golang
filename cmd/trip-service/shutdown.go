package main

import (
	"context"
	"fmt"
)

func closeWithinContext(ctx context.Context, app closer) error {
	done := make(chan struct{})
	go func() {
		app.Close(ctx)
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("service cleanup exceeded shutdown budget: %w", ctx.Err())
	}
}

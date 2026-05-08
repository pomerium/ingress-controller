package certificate_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/pomerium/ingress-controller/controllers/certificate"
)

func TestOperation(t *testing.T) {
	t.Parallel()
	t.Run("Active", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		assert.False(t, op.Active(), "should not be active before running")
		done := make(chan struct{})
		op.Start(func(_ context.Context) error {
			<-done
			return nil
		})
		assert.True(t, op.Active(), "should be active while running")
		close(done)
		_ = op.Wait()
		assert.False(t, op.Active(), "should not be active after running")
	})
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		assert.NoError(t, op.Error(), "should be no error before running")
		customErr := errors.New("custom")
		assert.ErrorIs(t, op.Run(func(_ context.Context) error { return customErr }), customErr)
		assert.ErrorIs(t, op.Error(), customErr)
		assert.NoError(t, op.Run(func(_ context.Context) error { return nil }))
		assert.NoError(t, op.Error())
		op.Start(func(ctx context.Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})
		assert.ErrorIs(t, op.Stop(), context.Canceled)
		assert.ErrorIs(t, op.Error(), context.Canceled)
	})
	t.Run("Reset", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		op.Reset()
		assert.False(t, op.Active())
		assert.NoError(t, op.Error())
		op.Start(func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
		op.Reset()
		assert.False(t, op.Active())
		assert.NoError(t, op.Error())
	})
	t.Run("Run", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		customErr := errors.New("custom")
		assert.NoError(t, op.Run(func(_ context.Context) error { return nil }))
		assert.ErrorIs(t, op.Run(func(_ context.Context) error { return customErr }), customErr)
	})
	t.Run("Start", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		op.Start(func(ctx context.Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})
		assert.ErrorIs(t, op.Stop(), context.Canceled)
	})
	t.Run("Stop", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		var done atomic.Bool
		op.Start(func(ctx context.Context) error {
			<-ctx.Done()
			time.Sleep(100 * time.Millisecond)
			done.Store(true)
			return nil
		})
		assert.NoError(t, op.Stop())
		assert.True(t, done.Load())
	})
	t.Run("StopNow", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		var done atomic.Bool
		op.Start(func(ctx context.Context) error {
			<-ctx.Done()
			time.Sleep(100 * time.Millisecond)
			done.Store(true)
			return nil
		})
		op.StopNow()
		assert.False(t, done.Load())
	})
	t.Run("Wait", func(t *testing.T) {
		t.Parallel()
		op := certificate.NewOperation()
		customErr := errors.New("custom")
		op.Start(func(_ context.Context) error { return customErr })
		assert.ErrorIs(t, op.Wait(), customErr)
	})
}

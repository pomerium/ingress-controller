package ingress

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startBatchCoordinator(t *testing.T, c *reconcileBatchCoordinator) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	return cancel
}

func TestReconcileBatchCoordinatorCoalescesBurstAfterQuietPeriod(t *testing.T) {
	var calls atomic.Int32
	flushed := make(chan time.Time, 2)
	c := newReconcileBatchCoordinator(30*time.Millisecond, 200*time.Millisecond, func(context.Context) error {
		calls.Add(1)
		flushed <- time.Now()
		return nil
	})
	startBatchCoordinator(t, c)

	c.Signal("test")
	time.Sleep(15 * time.Millisecond)
	c.Signal("test")
	lastSignal := time.Now()
	time.Sleep(15 * time.Millisecond)
	c.Signal("test")
	lastSignal = time.Now()

	select {
	case at := <-flushed:
		assert.GreaterOrEqual(t, at.Sub(lastSignal), 25*time.Millisecond)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("batch did not flush")
	}
	assert.Equal(t, int32(1), calls.Load())
}

func TestReconcileBatchCoordinatorBoundsContinuousEvents(t *testing.T) {
	flushed := make(chan time.Time, 2)
	c := newReconcileBatchCoordinator(40*time.Millisecond, 90*time.Millisecond, func(context.Context) error {
		flushed <- time.Now()
		return nil
	})
	startBatchCoordinator(t, c)

	started := time.Now()
	for i := 0; i < 8; i++ {
		c.Signal("test")
		time.Sleep(15 * time.Millisecond)
	}

	select {
	case at := <-flushed:
		assert.Less(t, at.Sub(started), 150*time.Millisecond)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("continuous events postponed the batch beyond maxWait")
	}
}

func TestReconcileBatchCoordinatorSerializesFlushes(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	started := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	c := newReconcileBatchCoordinator(time.Millisecond, 10*time.Millisecond, func(context.Context) error {
		current := active.Add(1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		return nil
	})
	startBatchCoordinator(t, c)

	c.Signal("test")
	require.Eventually(t, func() bool { return len(started) == 1 }, time.Second, time.Millisecond)
	c.Signal("test")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, len(started), "a second flush must not overlap the first")
	release <- struct{}{}
	require.Eventually(t, func() bool { return len(started) == 2 }, time.Second, time.Millisecond)
	release <- struct{}{}
	assert.Equal(t, int32(1), maxActive.Load())
}

func TestReconcileBatchCoordinatorReturnsFailureAndRetries(t *testing.T) {
	temporary := errors.New("temporary failure")
	var calls atomic.Int32
	c := newReconcileBatchCoordinator(time.Millisecond, 10*time.Millisecond, func(context.Context) error {
		if calls.Add(1) == 1 {
			return temporary
		}
		return nil
	})
	startBatchCoordinator(t, c)

	err := c.Submit(context.Background())
	require.ErrorIs(t, err, temporary)
	require.NoError(t, c.Submit(context.Background()))
	assert.Equal(t, int32(2), calls.Load())
}

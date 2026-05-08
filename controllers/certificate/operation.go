package certificate

import (
	"context"
	"sync"
	"sync/atomic"
)

type waitGroupWithError struct {
	sync.WaitGroup
	err atomic.Pointer[error]
}

type operation struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     *waitGroupWithError
}

// Active returns true if the operation is currently active.
func (o *operation) Active() bool {
	o.mu.Lock()
	active := o.wg != nil && o.wg.err.Load() == nil
	o.mu.Unlock()
	return active
}

// Error returns any current error.
func (o *operation) Error() error {
	var err error
	o.mu.Lock()
	if o.wg != nil {
		if e := o.wg.err.Load(); e != nil {
			err = *e
		}
	}
	o.mu.Unlock()
	return err
}

// Reset resets the state of the operation. If there is an operation running
// it will be stopped immediately.
func (o *operation) Reset() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	o.cancel = nil
	o.wg = nil
	o.mu.Unlock()
}

// Run runs the given function and waits for the result.
func (o *operation) Run(fn func(ctx context.Context) error) error {
	o.Start(fn)
	return o.Wait()
}

// Start starts an operation in a goroutine. If there is already an operation
// running it will be stopped immediately.
func (o *operation) Start(fn func(ctx context.Context) error) {
	ctx := context.Background()

	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	ctx, o.cancel = context.WithCancel(ctx)
	o.wg = new(waitGroupWithError)
	wg := o.wg
	o.mu.Unlock()

	wg.Go(func() {
		err := fn(ctx)
		wg.err.Store(new(err))
	})
}

// Stop stops an operation and waits for the result.
func (o *operation) Stop() error {
	o.mu.Lock()
	cancel := o.cancel
	wg := o.wg
	o.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if wg != nil {
		wg.Wait()
		if e := wg.err.Load(); e != nil {
			return *e
		}
	}
	return nil
}

// StopNow stops the operation immediately.
func (o *operation) StopNow() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	o.mu.Unlock()
}

// Wait waits for the result of the operation.
func (o *operation) Wait() error {
	o.mu.Lock()
	wg := o.wg
	o.mu.Unlock()

	if wg != nil {
		wg.Wait()
		if e := wg.err.Load(); e != nil {
			return *e
		}
	}
	return nil
}

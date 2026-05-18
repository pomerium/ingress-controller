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

// An Operation represents a long-running operation that can be stopped,
// monitored and restarted.
type Operation interface {
	// Active returns true if the operation is currently active.
	Active() bool
	// Error returns any current error.
	Error() error
	// Reset resets the state of the operation. If there is an operation running
	// it will be stopped immediately.
	Reset()
	// Run runs the given function and waits for the result.
	Run(fn func(ctx context.Context) error) error
	// Start starts an operation in a goroutine. If there is already an operation
	// running it will be stopped immediately.
	Start(fn func(ctx context.Context) error)
	// Stop stops an operation and waits for the result.
	Stop() error
	// StopNow stops the operation immediately.
	StopNow()
	// Wait waits for the result of the operation.
	Wait() error
}

type operation struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     *waitGroupWithError
}

// NewOperation creates a new Operation.
func NewOperation() Operation {
	return &operation{}
}

func (o *operation) Active() bool {
	o.mu.Lock()
	active := o.wg != nil && o.wg.err.Load() == nil
	o.mu.Unlock()
	return active
}

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

func (o *operation) Reset() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	o.cancel = nil
	o.wg = nil
	o.mu.Unlock()
}

func (o *operation) Run(fn func(ctx context.Context) error) error {
	o.Start(fn)
	return o.Wait()
}

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
		wg.err.Store(&err)
	})
}

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

func (o *operation) StopNow() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	o.mu.Unlock()
}

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

package util

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Config is a configuration abstraction
type Config[T any] interface {
	Clone() T
}

// RestartOnConfigChange allows to run a task that should be restarted when config changes.
type RestartOnConfigChange[T Config[T]] interface {
	// OnConfigUpdated may be called to provide updated configuration versions
	OnConfigUpdated(context.Context, T)
	// Run runs the task, restarting it when equal() returns false, by canceling task's execution context
	// if the task cannot complete within the given timeout threshold, Run would return an error
	// ensuring there are no unaccounted tasks left
	Run(ctx context.Context,
		equal func(prev, next T) bool,
		fn func(context.Context, T) error,
		shutdownTimeout time.Duration,
	) error
}

// NewRestartOnChange create a new instance
func NewRestartOnChange[T Config[T]]() RestartOnConfigChange[T] {
	return &restartOnChange[T]{
		updates: make(chan T),
	}
}

type restartOnChange[T Config[T]] struct {
	updates chan T
}

func (r *restartOnChange[T]) OnConfigUpdated(ctx context.Context, cfg T) {
	select {
	case <-ctx.Done():
		log.FromContext(ctx).Error(ctx.Err(), "failed to push config update into a queue")
	case r.updates <- cfg.Clone():
	}
}

func (r *restartOnChange[T]) Run(
	ctx context.Context,
	equal func(prev, next T) bool,
	fn func(ctx context.Context, cfg T) error,
	shutdownTimeout time.Duration,
) error {
	updates := make(chan T)
	defer close(updates)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { filterChanges(ctx, updates, r.updates, equal); return nil })
	eg.Go(func() error { return runTasks(ctx, updates, fn, shutdownTimeout) })

	return eg.Wait()
}

func runTasks[T any](ctx context.Context,
	updates chan T,
	fn func(context.Context, T) error,
	shutdownTimeout time.Duration,
) error {
	var cancel context.CancelFunc = func() {}
	defer cancel()

	logger := log.FromContext(ctx).V(1)
	logger.Info("starting task loop")
	var done chan error
	for {
		select {
		case <-ctx.Done():
			logger.Info("context canceled, quit")
			return nil
		case cfg := <-updates:
			cancel()
			logger.Info("canceling and waiting for previous task to finish...")
			if err := waitCompletion(ctx, done, shutdownTimeout); err != nil {
				logger.Error(err, "waiting for task to finish")
				return fmt.Errorf("waiting for task completion: %w", err)
			}
			logger.Info("restarting new task", "config", cfg)
			cancel, done = goRunTask(ctx, cfg, fn)
		case err := <-done:
			logger.Error(err, "task quit unexpectedly")
			return fmt.Errorf("task quit: %w", err)
		}
	}
}

func waitCompletion(ctx context.Context, done chan error, timeout time.Duration) error {
	if done == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func goRunTask[T any](ctx context.Context, cfg T, fn func(context.Context, T) error) (context.CancelFunc, chan error) {
	ctx, cancel := context.WithCancel(ctx)

	done := make(chan error)
	go func() {
		done <- fn(ctx, cfg)
		close(done)
	}()

	return cancel, done
}

func filterChanges[T any](ctx context.Context, dst, src chan T, equal func(prev, next T) bool) {
	var cfg T
	for {
		select {
		case <-ctx.Done():
			return
		case next, ok := <-src:
			if !ok {
				return
			}
			if equal(cfg, next) {
				continue
			}
			cfg = next
			select {
			case <-ctx.Done():
				return
			case dst <- cfg:
			}
		}
	}
}

package controllers

import (
	"context"
)

type once struct {
	execCtx chan context.Context
	result  chan error
}

func newOnce(runnable func(ctx context.Context) error, onError func()) *once {
	o := &once{
		execCtx: make(chan context.Context),
		result:  make(chan error),
	}
	go func() {
		ctx := <-o.execCtx
		err := runnable(ctx)
		if err != nil {
			onError()
		}
		o.result <- err
		close(o.result)
	}()
	return o
}

func (o *once) yield(ctx context.Context) error {
	select {
	case err := <-o.result:
		return err
	case o.execCtx <- ctx:
		return o.wait(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (o *once) wait(ctx context.Context) error {
	select {
	case err := <-o.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

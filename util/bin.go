package util

import "context"

type key[T any] struct{}

type bin[T any] struct {
	entries []T
}

// WithBin enables collector of objects that's stored in context
// that may be used to collect i.e. some warnings that do not cause errors
// or maybe document some defaults that were applied
func WithBin[T any](ctx context.Context) context.Context {
	k := key[T]{}
	_, ok := ctx.Value(k).(*bin[T])
	if ok {
		return ctx
	}
	return context.WithValue(ctx, k, new(bin[T]))
}

// Add attaches an entry to the collector
func Add[T any](ctx context.Context, entries ...T) {
	collector, ok := ctx.Value(key[T]{}).(*bin[T])
	if !ok {
		return
	}
	collector.entries = append(collector.entries, entries...)
}

// Get returns all entries attached to the collector
func Get[T any](ctx context.Context) []T {
	collector, ok := ctx.Value(key[T]{}).(*bin[T])
	if !ok {
		return nil
	}
	return collector.entries
}

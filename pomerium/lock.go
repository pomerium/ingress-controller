package pomerium

import (
	"context"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
)

type syncLock struct {
	Reconciler
	lock chan struct{}
}

// WithLock is a reconciler that only performs a single sync at a time
// which is necessary as we must have individual controllers for Ingress and Settings CRDs
func WithLock(reconciler Reconciler) Reconciler {
	return &syncLock{
		Reconciler: reconciler,
		lock:       make(chan struct{}, 1),
	}
}

func (s *syncLock) Lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.lock <- struct{}{}:
		return nil
	}
}

func (s *syncLock) Unlock() {
	<-s.lock
}

// Upsert should update or create the pomerium routes corresponding to this ingress
func (s *syncLock) Upsert(ctx context.Context, ic *model.IngressConfig, cfg *model.Config) (changes bool, err error) {
	if err := s.Lock(ctx); err != nil {
		return false, err
	}
	defer s.Unlock()
	return s.Reconciler.Upsert(ctx, ic, cfg)
}

// Set configuration to match provided ingresses
func (s *syncLock) Set(ctx context.Context, ics []*model.IngressConfig, cfg *model.Config) (changes bool, err error) {
	if err := s.Lock(ctx); err != nil {
		return false, err
	}
	defer s.Unlock()
	return s.Reconciler.Set(ctx, ics, cfg)
}

// Delete should delete pomerium routes corresponding to this ingress name
func (s *syncLock) Delete(ctx context.Context, namespacedName types.NamespacedName) error {
	if err := s.Lock(ctx); err != nil {
		return err
	}
	defer s.Unlock()
	return s.Reconciler.Delete(ctx, namespacedName)
}

// SetConfig updates just the shared settings
func (s *syncLock) SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error) {
	if err := s.Lock(ctx); err != nil {
		return false, err
	}
	defer s.Unlock()
	return s.Reconciler.SetConfig(ctx, cfg)
}

package ctrl

import (
	"context"
	"sync"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/pomerium/pomerium/config"
)

// InMemoryConfigSource represents bootstrap config source
type InMemoryConfigSource struct {
	mu        sync.Mutex
	cfg       *config.Config
	listeners []config.ChangeListener
}

var (
	_ = config.Source(new(InMemoryConfigSource))
)

var (
	cmpOpts = []cmp.Option{
		cmpopts.IgnoreUnexported(config.Options{}),
		cmpopts.EquateEmpty(),
	}
)

// SetConfig updates the underlying configuration
// it returns true if configuration was updated
// and informs config change listeners in case there was a change
func (src *InMemoryConfigSource) SetConfig(ctx context.Context, cfg *config.Config) bool {
	src.mu.Lock()
	defer src.mu.Unlock()

	if changed := !cmp.Equal(cfg, src.cfg, cmpOpts...); !changed {
		return false
	}

	src.cfg = cfg.Clone()

	for _, l := range src.listeners {
		l(ctx, src.cfg)
	}

	return true
}

// GetConfig implements config.Source
func (src *InMemoryConfigSource) GetConfig() *config.Config {
	src.mu.Lock()
	defer src.mu.Unlock()

	if src.cfg == nil {
		panic("should not be called prior to initial config available")
	}

	return src.cfg
}

// OnConfigChange implements config.Source
func (src *InMemoryConfigSource) OnConfigChange(_ context.Context, l config.ChangeListener) {
	src.mu.Lock()
	src.listeners = append(src.listeners, l)
	src.mu.Unlock()
}

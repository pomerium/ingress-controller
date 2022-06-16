package ctrl

import (
	"context"
	"sync"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/pomerium/pomerium/config"
)

// ConfigSource represents bootstrap config source
type ConfigSource struct {
	base      config.Config
	opts      *config.Options
	listeners []config.ChangeListener
	sync.Mutex
}

var (
	_ = config.Source(new(ConfigSource))
)

// NewConfigSource creates base config source
func NewConfigSource() (*ConfigSource, error) {
	cfg := config.Config{
		Options: config.NewDefaultOptions(),
	}
	if err := cfg.AllocatePorts(); err != nil {
		return nil, err
	}
	return &ConfigSource{
		base: cfg,
	}, nil
}

var (
	cmpOpts = cmpopts.IgnoreUnexported(config.Options{})
)

// SetOptions updates the underlying configuration
// it returns true if configuration was updated
// and informs config change listeners in case there was a change
func (cfg *ConfigSource) SetOptions(ctx context.Context, opts config.Options) bool {
	cfg.Lock()
	defer cfg.Unlock()

	changed := true
	if cfg.opts != nil {
		changed = !cmp.Equal(opts, *cfg.opts, cmpOpts)
	}
	if !changed {
		return false
	}

	cfg.opts = &opts
	c := cfg.getConfigLocked()

	for _, l := range cfg.listeners {
		l(ctx, c)
	}

	return true
}

// GetConfig implements config.Source
func (cfg *ConfigSource) GetConfig() *config.Config {
	cfg.Lock()
	defer cfg.Unlock()

	return cfg.getConfigLocked()
}

func (cfg *ConfigSource) getConfigLocked() *config.Config {
	c := cfg.base
	c.Options = cfg.opts
	return &c
}

// OnConfigChange implements config.Source
func (cfg *ConfigSource) OnConfigChange(_ context.Context, l config.ChangeListener) {
	cfg.Lock()
	cfg.listeners = append(cfg.listeners, l)
	cfg.Unlock()
}

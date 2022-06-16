package ctrl

import (
	"context"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/pomerium/config"
	pomerium_cmd "github.com/pomerium/pomerium/pkg/cmd/pomerium"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
)

var (
	_ = pomerium.ConfigReconciler(new(Runner))
)

// Runner implements pomerium control loop
type Runner struct {
	cfg *ConfigSource
	sync.Once
	ready chan struct{}
}

// WaitForConfig waits until initial configuration is available
func (r *Runner) WaitForConfig(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ready:
	}
	return nil
}

func (r *Runner) readyToRun() {
	close(r.ready)
}

// GetConfig returns current configuration snapshot
func (r *Runner) GetConfig() *config.Config {
	return r.cfg.GetConfig()
}

// SetConfig updates just the shared config settings
func (r *Runner) SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error) {
	opts, err := Make(cfg)
	if err != nil {
		return false, fmt.Errorf("transform config: %w", err)
	}

	changed := r.cfg.SetOptions(ctx, *opts)
	r.Once.Do(r.readyToRun)

	return changed, nil
}

// NewPomeriumRunner creates new pomerium command and control
func NewPomeriumRunner() (*Runner, error) {
	cfg, err := NewConfigSource()
	if err != nil {
		return nil, err
	}

	return &Runner{
		cfg:   cfg,
		ready: make(chan struct{}),
	}, nil
}

// Run starts pomerium once config is available
func (r *Runner) Run(ctx context.Context) error {
	if err := r.WaitForConfig(ctx); err != nil {
		return fmt.Errorf("waiting for pomerium bootstrap config: %w", err)
	}

	log.FromContext(ctx).Info("got bootstrap config, starting pomerium...", "opts", r.cfg.GetConfig().Options)

	return pomerium_cmd.Run(ctx, r.cfg)
}

package ctrl

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/pomerium/config"
	pomerium_cmd "github.com/pomerium/pomerium/pkg/cmd/pomerium"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
)

var _ = pomerium.ConfigReconciler(new(Runner))

// Runner implements pomerium control loop
type Runner struct {
	src  *InMemoryConfigSource
	base config.Config
	cfg  config.Config
	sync.Once
	ready chan struct{}

	syncAPIHost        string
	bootstrapIngresses map[types.NamespacedName][]config.Policy
}

// waitForConfig waits until initial configuration is available
func (r *Runner) waitForConfig(ctx context.Context) error {
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
	return r.src.GetConfig()
}

// SetConfig updates just the shared config settings
func (r *Runner) SetConfig(ctx context.Context, src *model.Config) (changes bool, err error) {
	dst := r.base.Clone()

	if err := Apply(ctx, dst.Options, src); err != nil {
		return false, fmt.Errorf("transform config: %w", err)
	}

	// If using all-in-one mode together with the sync API, we may need to bootstrap
	// more of the settings than usual. The Enterprise API can't be used to control
	// many of the core settings (authenticate URL, IdP settings, etc.).
	if r.syncAPIHost != "" {
		if err := ApplyAdditional(ctx, dst.Options, src); err != nil {
			return false, fmt.Errorf("additional settings: %w")
		}
	}

	r.cfg = *dst
	changed := r.updateConfig(ctx)
	r.Once.Do(r.readyToRun)

	return changed, nil
}

func (r *Runner) updateConfig(ctx context.Context) (changes bool) {
	cfg := r.cfg.Clone()
	r.applyBootstrapIngresses(cfg)

	//log.FromContext(ctx).V(1).Info("bootstrap routes")

	return r.src.SetConfig(ctx, cfg)
}

// XXX: pull this logic out into bootstrap.go?

// Upsert adds a bootstrap route for this ingress if it matches the sync API URL.
func (r *Runner) Upsert(ctx context.Context, ic *model.IngressConfig) (changes bool, err error) {
	if !r.isBootstrapIngress(ic) {
		delete(r.bootstrapIngresses, ic.GetIngressNamespacedName())
	} else if err := r.addBootstrapIngress(ctx, ic); err != nil {
		return false, err
	}

	return r.updateConfig(ctx), nil
}

// Set adds bootstrap routes for any ingresses matching the sync API URL.
func (r *Runner) Set(ctx context.Context, ics []*model.IngressConfig) (changes bool, err error) {
	clear(r.bootstrapIngresses)

	if err := r.addBootstrapIngress(ctx, ics...); err != nil {
		return false, err
	}

	return r.updateConfig(ctx), nil
}

// Delete removes any bootstrap routes corresponding to the given ingress name.
func (r *Runner) Delete(ctx context.Context, namespacedName types.NamespacedName) (changes bool, err error) {
	delete(r.bootstrapIngresses, namespacedName)
	return r.updateConfig(ctx), nil
}

func (r *Runner) isBootstrapIngress(ic *model.IngressConfig) bool {
	for _, rule := range ic.Spec.Rules {
		if rule.Host == r.syncAPIHost {
			return true
		}
	}
	return false
}

func (r *Runner) addBootstrapIngress(ctx context.Context, ics ...*model.IngressConfig) error {
	for _, ic := range ics {
		if !r.isBootstrapIngress(ic) {
			continue
		}
		routes, err := pomerium.IngressToRoutes(ctx, ic)
		if err != nil {
			return err
		}
		log.FromContext(ctx).V(1).Info("addBootstrapIngress found bootstrap ingress")
		r.bootstrapIngresses[ic.GetIngressNamespacedName()] = routes
	}
	return nil
}

func (r *Runner) applyBootstrapIngresses(cfg *config.Config) {
	for _, routes := range r.bootstrapIngresses {
		cfg.Options.Routes = append(cfg.Options.Routes, routes...)
	}
}

// NewPomeriumRunner creates new pomerium command and control
func NewPomeriumRunner(base config.Config, listener config.ChangeListener, syncAPIURL string) (*Runner, error) {
	var syncAPIHost string
	if syncAPIURL != "" {
		u, err := url.Parse(syncAPIURL)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse sync API URL: %w", err)
		}
		syncAPIHost = u.Host
	}
	return &Runner{
		base: base,
		src: &InMemoryConfigSource{
			listeners: []config.ChangeListener{listener},
		},
		ready:              make(chan struct{}),
		syncAPIHost:        syncAPIHost,
		bootstrapIngresses: make(map[types.NamespacedName][]config.Policy),
	}, nil
}

// Run starts pomerium once config is available
func (r *Runner) Run(ctx context.Context) error {
	if err := r.waitForConfig(ctx); err != nil {
		return fmt.Errorf("waiting for pomerium bootstrap config: %w", err)
	}

	log.FromContext(ctx).V(1).Info("got bootstrap config, starting pomerium...", "cfg", r.src.GetConfig())

	return pomerium_cmd.Run(ctx, r.src)
}

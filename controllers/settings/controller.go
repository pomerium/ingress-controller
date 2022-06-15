package settings

import (
	context "context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/controllers/deps"
	"github.com/pomerium/ingress-controller/controllers/reporter"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
)

type settingsController struct {
	// name of a settings object to watch, all others would be ignored
	name types.NamespacedName
	// Client is k8s apiserver client
	client.Client
	// PomeriumReconciler updates Pomerium service configuration
	pomerium.ConfigReconciler
	// Registry is used to keep track of dependencies between objects
	model.Registry
	// Scheme keeps track of registered object types and kinds
	*runtime.Scheme
	// MultiPomeriumStatusReporter is used to report when settings are updated
	reporter.MultiPomeriumStatusReporter
}

// NewSettingsController creates and registers a new controller for
// a given settings object, as we can only watch single settings
func NewSettingsController(
	mgr ctrl.Manager,
	pcr pomerium.ConfigReconciler,
	name types.NamespacedName,
) error {
	stc := &settingsController{
		name:             name,
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		Registry:         model.NewRegistry(),
		ConfigReconciler: pcr,
		MultiPomeriumStatusReporter: []reporter.PomeriumReporter{
			&reporter.SettingsEventReporter{
				EventRecorder: mgr.GetEventRecorderFor("pomerium"),
				SettingsReporter: reporter.SettingsReporter{
					NamespacedName: name,
					Client:         mgr.GetClient(),
				},
			},
			&reporter.SettingsStatusReporter{
				SettingsReporter: reporter.SettingsReporter{
					NamespacedName: name,
					Client:         mgr.GetClient(),
				},
			},
			&reporter.SettingsLogReporter{},
		},
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(new(icsv1.Settings)).Build(stc)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	for _, o := range []struct {
		client.Object
		mapFn func(model.Registry, string) handler.MapFunc
	}{
		{new(corev1.Secret), deps.GetDependantMapFunc},
	} {
		gvk, err := apiutil.GVKForObject(o.Object, mgr.GetScheme())
		if err != nil {
			return fmt.Errorf("cannot get kind: %w", err)
		}

		if err := c.Watch(
			&source.Kind{Type: o.Object},
			handler.EnqueueRequestsFromMapFunc(o.mapFn(stc.Registry, gvk.Kind))); err != nil {
			return fmt.Errorf("watching %s: %w", gvk.String(), err)
		}
	}
	return nil
}

// Reconcile syncs Settings CRD with pomerium databroker
func (c *settingsController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.NamespacedName != c.name {
		return ctrl.Result{}, nil
	}

	cfg, err := FetchConfig(ctx, c.Client, c.name)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("get settings: %w", err)
	}

	c.updateDependencies(cfg)

	updated, err := c.SetConfig(ctx, cfg)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("set config: %w", err)
	}
	if updated {
		c.SettingsUpdated(ctx)
	}

	return ctrl.Result{}, nil
}

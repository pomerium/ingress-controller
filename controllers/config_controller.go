// Package controllers contains k8s reconciliation controllers
package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/types"
	runtime_ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/pomerium/pkg/grpc/databroker"

	"github.com/pomerium/ingress-controller/controllers/gateway"
	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/controllers/reporter"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/pomerium"
	health_ctrl "github.com/pomerium/ingress-controller/util/health"
)

const (
	leaseDuration = time.Second * 30
)

var (
	_ = databroker.LeaserHandler(new(Controller))

	errWaitingForLease = errors.New("waiting for databroker lease")
)

// Controller runs Pomerium configuration reconciliation controllers
// for Ingress and Pomerium Settings CRD objects, if specified
type Controller struct {
	pomerium.IngressReconciler
	pomerium.GatewayReconciler
	pomerium.ConfigReconciler
	databroker.DataBrokerServiceClient
	MgrOpts runtime_ctrl.Options
	// IngressCtrlOpts are the ingress controller options
	IngressCtrlOpts []ingress.Option
	// GatewayControllerConfig is the Gateway controller config
	GatewayControllerConfig *gateway.ControllerConfig
	// GlobalSettings if provided, will also reconcile configuration options
	GlobalSettings *types.NamespacedName

	running int32
}

// Run runs controller using lease
func (c *Controller) Run(ctx context.Context) error {
	leaser := databroker.NewLeaser("ingress-controller", leaseDuration, c)
	return leaser.Run(ctx)
}

// GetDataBrokerServiceClient implements databroker.LeaseHandler
func (c *Controller) GetDataBrokerServiceClient() databroker.DataBrokerServiceClient {
	return c.DataBrokerServiceClient
}

// RunLeased implements databroker.LeaseHandler
func (c *Controller) RunLeased(ctx context.Context) (err error) {
	defer c.setRunning(false)

	cfg, err := runtime_ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get k8s api config: %w", err)
	}
	mgr, err := runtime_ctrl.NewManager(cfg, c.MgrOpts)
	if err != nil {
		return fmt.Errorf("unable to create controller manager: %w", err)
	}

	if err = ingress.NewIngressController(mgr, c.IngressReconciler, c.getIngressOpts(mgr)...); err != nil {
		return fmt.Errorf("create ingress controller: %w", err)
	}
	if c.GlobalSettings != nil {
		if err = settings.NewSettingsController(mgr, c.ConfigReconciler, *c.GlobalSettings, "pomerium-crd", true, health_ctrl.SettingsReconciler); err != nil {
			return fmt.Errorf("create settings controller: %w", err)
		}
	} else {
		log.FromContext(ctx).V(1).Info("no Pomerium CRD")
	}

	if c.GatewayControllerConfig != nil {
		err := gateway.NewControllers(ctx, mgr, c.GatewayReconciler, *c.GatewayControllerConfig)
		if err != nil {
			return err
		}
	}

	c.setRunning(true)
	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("running controller: %w", err)
	}
	return nil
}

func (c *Controller) setRunning(running bool) {
	if running {
		atomic.StoreInt32(&c.running, 1)
	} else {
		atomic.StoreInt32(&c.running, 0)
	}
}

// ReadyzCheck reports whether controller is ready
func (c *Controller) ReadyzCheck(_ *http.Request) error {
	val := atomic.LoadInt32(&c.running)
	if val == 0 {
		return errWaitingForLease
	}
	return nil
}

func (c *Controller) getIngressOpts(mgr runtime_ctrl.Manager) []ingress.Option {
	if c.GlobalSettings == nil {
		return c.IngressCtrlOpts
	}

	rep := reporter.SettingsReporter{
		NamespacedName: *c.GlobalSettings,
		Client:         mgr.GetClient(),
	}

	return append(c.IngressCtrlOpts, ingress.WithIngressStatusReporter(
		&reporter.IngressSettingsReporter{
			SettingsReporter: rep,
		},
		&reporter.IngressSettingsEventReporter{
			EventRecorder:    mgr.GetEventRecorderFor("pomerium-ingress"),
			SettingsReporter: rep,
		}))
}

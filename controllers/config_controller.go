//Package controllers contains k8s reconciliation controllers
package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/types"
	runtime_ctrl "sigs.k8s.io/controller-runtime"

	"github.com/pomerium/pomerium/pkg/grpc/databroker"

	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/pomerium"
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
	pomerium.Reconciler
	databroker.DataBrokerServiceClient
	MgrOpts runtime_ctrl.Options
	// IngressCtrlOpts are the ingress controller options
	IngressCtrlOpts []ingress.Option
	// GlobalSettings if provided, will also reconcile configuration options
	GlobalSettings *types.NamespacedName

	running int32
}

// Run runs controller using lease
func (c *Controller) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		leaser := databroker.NewLeaser("ingress-controller", leaseDuration, c)
		return leaser.Run(ctx)
	})
	/* TODO
	eg.Go(func() error {
		return s.runHealthz(ctx, healthz.NamedCheck("acquire databroker lease", ct.ReadyzCheck))
	})
	*/
	return eg.Wait()
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

// GetDataBrokerServiceClient implements databroker.LeaseHandler
func (c *Controller) GetDataBrokerServiceClient() databroker.DataBrokerServiceClient {
	return c.DataBrokerServiceClient
}

// RunLeased implements databroker.LeaseHandler
func (c *Controller) RunLeased(ctx context.Context) error {
	defer c.setRunning(false)

	cfg, err := runtime_ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get k8s api config: %w", err)
	}
	mgr, err := runtime_ctrl.NewManager(cfg, c.MgrOpts)
	if err != nil {
		return fmt.Errorf("unable to create controller manager: %w", err)
	}

	if err = ingress.NewIngressController(mgr, c.Reconciler, c.IngressCtrlOpts...); err != nil {
		return fmt.Errorf("create ingress controller: %w", err)
	}
	if c.GlobalSettings != nil {
		if err = settings.NewSettingsController(mgr, c.Reconciler, *c.GlobalSettings); err != nil {
			return fmt.Errorf("create settings controller: %w", err)
		}
	}

	c.setRunning(true)
	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("running controller: %w", err)
	}
	return nil
}

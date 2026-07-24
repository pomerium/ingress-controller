package cmd

import (
	"fmt"
	"time"

	validate "github.com/go-playground/validator/v10"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/controllers/gateway"
	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/util"
)

type ingressControllerOpts struct {
	ClassName               string `validate:"required"`
	GatewayAPIEnabled       bool
	GatewayClassName        string `validate:"required"`
	AnnotationPrefix        string `validate:"required"`
	Namespaces              []string
	UpdateStatusFromService string        ``
	GlobalSettings          string        `validate:"required"`
	ReconcileBatchWindow    time.Duration `validate:"gte=0"`
	ReconcileBatchMaxWait   time.Duration `validate:"gte=0"`
}

const (
	ingressClassControllerName = "name"
	experimentalGatewayAPI     = "experimental-gateway-api"
	gatewayClassControllerName = "gateway-class-controller-name"
	annotationPrefix           = "prefix"
	namespaces                 = "namespaces"
	sharedSecret               = "shared-secret"
	updateStatusFromService    = "update-status-from-service"
	globalSettings             = "pomerium-config"
	reconcileBatchWindow       = "reconcile-batch-window"
	reconcileBatchMaxWait      = "reconcile-batch-max-wait"
)

func (s *ingressControllerOpts) setupFlags(flags *pflag.FlagSet) {
	flags.StringVar(&s.ClassName, ingressClassControllerName, ingress.DefaultClassControllerName, "IngressClass controller name")
	flags.BoolVar(&s.GatewayAPIEnabled, experimentalGatewayAPI, false, "experimental support for the Kubernetes Gateway API")
	flags.StringVar(&s.GatewayClassName, gatewayClassControllerName, gateway.DefaultClassControllerName, "GatewayClass controller name")
	flags.StringVar(&s.AnnotationPrefix, annotationPrefix, ingress.DefaultAnnotationPrefix, "Ingress annotation prefix")
	flags.StringSliceVar(&s.Namespaces, namespaces, nil, "namespaces to watch, or none to watch all namespaces")
	flags.StringVar(&s.UpdateStatusFromService, updateStatusFromService, "", "update ingress status from given service status (pomerium-proxy)")
	flags.StringVar(&s.GlobalSettings, globalSettings, "",
		fmt.Sprintf("namespace/name to a resource of type %s/Settings", icsv1.GroupVersion.Group))
	flags.DurationVar(&s.ReconcileBatchWindow, reconcileBatchWindow, 0,
		"wait for this quiet period before applying real ingress changes as one batch (disabled by default)")
	flags.DurationVar(&s.ReconcileBatchMaxWait, reconcileBatchMaxWait, 5*time.Second,
		"maximum time to delay a batch while ingress changes continue arriving")
}

func (s *ingressControllerOpts) Validate() error {
	if err := validate.New().Struct(s); err != nil {
		return err
	}
	if s.ReconcileBatchWindow > 0 && s.ReconcileBatchMaxWait < s.ReconcileBatchWindow {
		return fmt.Errorf("--%s must be greater than or equal to --%s", reconcileBatchMaxWait, reconcileBatchWindow)
	}
	return nil
}

func (s *ingressControllerOpts) getGlobalSettings() (*types.NamespacedName, error) {
	if s.GlobalSettings == "" {
		return nil, nil
	}

	name, err := util.ParseNamespacedName(s.GlobalSettings, util.WithClusterScope())
	if err != nil {
		return nil, fmt.Errorf("%s=%s: %w", globalSettings, s.GlobalSettings, err)
	}
	return name, nil
}

func (s *ingressControllerOpts) getIngressControllerOptions() ([]ingress.Option, error) {
	opts := []ingress.Option{
		ingress.WithNamespaces(s.Namespaces),
		ingress.WithAnnotationPrefix(s.AnnotationPrefix),
		ingress.WithControllerName(s.ClassName),
	}
	if s.ReconcileBatchWindow > 0 {
		opts = append(opts, ingress.WithAdaptiveReconcileBatching(
			s.ReconcileBatchWindow,
			s.ReconcileBatchMaxWait,
		))
	}
	if name, err := s.getGlobalSettings(); err != nil {
		return nil, err
	} else if name != nil {
		opts = append(opts, ingress.WithGlobalSettings(*name))
	}
	if s.UpdateStatusFromService != "" {
		name, err := util.ParseNamespacedName(s.UpdateStatusFromService)
		if err != nil {
			return nil, fmt.Errorf("update status from service: %q: %w", s.UpdateStatusFromService, err)
		}
		opts = append(opts, ingress.WithUpdateIngressStatusFromService(*name))
	}
	return opts, nil
}

func (s *ingressControllerOpts) getGatewayControllerConfig() (*gateway.ControllerConfig, error) {
	if !s.GatewayAPIEnabled {
		return nil, nil
	}

	cfg := &gateway.ControllerConfig{
		ControllerName: s.GatewayClassName,
	}
	if s.UpdateStatusFromService != "" {
		name, err := util.ParseNamespacedName(s.UpdateStatusFromService)
		if err != nil {
			return cfg, fmt.Errorf("update status from service: %q: %w", s.UpdateStatusFromService, err)
		}
		cfg.ServiceName = *name
	}
	return cfg, nil
}

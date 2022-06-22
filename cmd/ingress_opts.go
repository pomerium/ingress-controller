package cmd

import (
	"fmt"

	validate "github.com/go-playground/validator/v10"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/util"
)

type ingressControllerOpts struct {
	className               string `validate:"required"`
	annotationPrefix        string `validate:"required"`
	namespaces              []string
	disableCertCheck        bool
	updateStatusFromService string `validate:"required"`
	globalSettings          string
}

const (
	ingressClassControllerName = "name"
	annotationPrefix           = "prefix"
	namespaces                 = "namespaces"
	sharedSecret               = "shared-secret"
	debug                      = "debug"
	updateStatusFromService    = "update-status-from-service"
	disableCertCheck           = "disable-cert-check"
	globalSettings             = "global-settings"
)

func (s *ingressControllerOpts) setupFlags(flags *pflag.FlagSet) {
	flags.StringVar(&s.className, ingressClassControllerName, ingress.DefaultClassControllerName, "IngressClass controller name")
	flags.StringVar(&s.annotationPrefix, annotationPrefix, ingress.DefaultAnnotationPrefix, "Ingress annotation prefix")
	flags.StringSliceVar(&s.namespaces, namespaces, nil, "namespaces to watch, or none to watch all namespaces")
	flags.StringVar(&s.updateStatusFromService, updateStatusFromService, "", "update ingress status from given service status (pomerium-proxy)")
	flags.BoolVar(&s.disableCertCheck, disableCertCheck, false, "disables certificate check")
	flags.StringVar(&s.globalSettings, globalSettings, "",
		fmt.Sprintf("namespace/name to a resource of type %s/Settings", icsv1.GroupVersion.Group))
}

func (s *ingressControllerOpts) Validate() error {
	return validate.New().Struct(s)
}

func (s *ingressControllerOpts) getGlobalSettings() (*types.NamespacedName, error) {
	if s.globalSettings == "" {
		return nil, nil
	}

	name, err := util.ParseNamespacedName(s.globalSettings)
	if err != nil {
		return nil, fmt.Errorf("%s=%s: %w", globalSettings, s.globalSettings, err)
	}
	return name, nil
}

func (s *ingressControllerOpts) getIngressControllerOptions() ([]ingress.Option, error) {
	opts := []ingress.Option{
		ingress.WithNamespaces(s.namespaces),
		ingress.WithAnnotationPrefix(s.annotationPrefix),
		ingress.WithControllerName(s.className),
	}
	if s.disableCertCheck {
		opts = append(opts, ingress.WithDisableCertCheck())
	}
	if name, err := s.getGlobalSettings(); err != nil {
		return nil, err
	} else if name != nil {
		opts = append(opts, ingress.WithGlobalSettings(*name))
	}
	if s.updateStatusFromService != "" {
		name, err := util.ParseNamespacedName(s.updateStatusFromService)
		if err != nil {
			return nil, fmt.Errorf("update status from service: %q: %w", s.updateStatusFromService, err)
		}
		opts = append(opts, ingress.WithUpdateIngressStatusFromService(*name))
	}
	return opts, nil
}

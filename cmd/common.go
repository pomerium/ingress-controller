// Package cmd implements top level commands
package cmd

import (
	"fmt"
	"time"

	"github.com/iancoleman/strcase"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

const (
	defaultGRPCTimeout = time.Minute
)

const (
	webhookPort                = "webhook-port"
	metricsBindAddress         = "metrics-bind-address"
	healthProbeBindAddress     = "health-probe-bind-address"
	className                  = "name"
	annotationPrefix           = "prefix"
	databrokerServiceURL       = "databroker-service-url"
	databrokerTLSCAFile        = "databroker-tls-ca-file"
	databrokerTLSCA            = "databroker-tls-ca"
	tlsInsecureSkipVerify      = "databroker-tls-insecure-skip-verify"
	tlsOverrideCertificateName = "databroker-tls-override-certificate-name"
	namespaces                 = "namespaces"
	sharedSecret               = "shared-secret"
	debug                      = "debug"
	updateStatusFromService    = "update-status-from-service"
	disableCertCheck           = "disable-cert-check"
	globalSettings             = "global-settings"
	secrets                    = "secrets"
)

func envName(name string) string {
	return strcase.ToScreamingSnake(name)
}

func setupLogger(debug bool) {
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}
	opts := zap.Options{
		Development:     debug,
		Level:           level,
		StacktraceLevel: zapcore.DPanicLevel,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
}

func getScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, apply := range []struct {
		name string
		fn   func(*runtime.Scheme) error
	}{
		{"core", clientgoscheme.AddToScheme},
		{"settings", icsv1.AddToScheme},
	} {
		if err := apply.fn(scheme); err != nil {
			return nil, fmt.Errorf("%s: %w", apply.name, err)
		}
	}
	return scheme, nil
}

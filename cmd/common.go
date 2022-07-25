// Package cmd implements top level commands
package cmd

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/iancoleman/strcase"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	webhookPort            = "webhook-port"
	metricsBindAddress     = "metrics-bind-address"
	healthProbeBindAddress = "health-probe-bind-address"
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

func viperWalk(flags *pflag.FlagSet) error {
	v := viper.New()
	var errs *multierror.Error
	flags.VisitAll(func(f *pflag.Flag) {
		if err := v.BindEnv(f.Name, envName(f.Name)); err != nil {
			errs = multierror.Append(errs, err)
			return
		}

		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			errs = multierror.Append(errs, flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
	return errs.ErrorOrNil()
}

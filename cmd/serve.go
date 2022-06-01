// Package cmd implements top level commands
package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/server/healthz"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/pomerium"
	"github.com/pomerium/ingress-controller/util"
)

const (
	defaultGRPCTimeout = time.Minute
	leaseDuration      = time.Second * 30
)

var (
	errWaitingForLease = errors.New("waiting for databroker lease")
)

type serveCmd struct {
	metricsAddr      string
	webhookPort      int
	probeAddr        string
	className        string
	annotationPrefix string
	namespaces       []string

	databrokerServiceURL       string
	tlsCAFile                  string
	tlsCA                      []byte
	tlsInsecureSkipVerify      bool
	tlsOverrideCertificateName string

	sharedSecret string

	disableCertCheck bool

	updateStatusFromService string
	globalSettings          string

	debug bool

	cobra.Command
	pomerium.Reconciler
}

// ServeCommand creates command to run ingress controller
func ServeCommand() (*cobra.Command, error) {
	cmd := serveCmd{
		Command: cobra.Command{
			Use:   "serve",
			Short: "run ingress controller",
		}}
	cmd.RunE = cmd.exec
	if err := cmd.setupFlags(); err != nil {
		return nil, err
	}
	return &cmd.Command, nil
}

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
)

func envName(name string) string {
	return strcase.ToScreamingSnake(name)
}

func (s *serveCmd) setupFlags() error {
	flags := s.PersistentFlags()
	flags.IntVar(&s.webhookPort, webhookPort, 9443, "webhook port")
	flags.StringVar(&s.metricsAddr, metricsBindAddress, ":8080", "The address the metric endpoint binds to.")
	flags.StringVar(&s.probeAddr, healthProbeBindAddress, ":8081", "The address the probe endpoint binds to.")
	flags.StringVar(&s.className, className, ingress.DefaultClassControllerName, "IngressClass controller name")
	flags.StringVar(&s.annotationPrefix, annotationPrefix, ingress.DefaultAnnotationPrefix, "Ingress annotation prefix")
	flags.StringVar(&s.databrokerServiceURL, databrokerServiceURL, "http://localhost:5443",
		"the databroker service url")
	flags.StringVar(&s.tlsCAFile, databrokerTLSCAFile, "", "tls CA file path")
	flags.BytesBase64Var(&s.tlsCA, databrokerTLSCA, nil, "base64 encoded tls CA")
	flags.BoolVar(&s.tlsInsecureSkipVerify, tlsInsecureSkipVerify, false,
		"disable remote hosts TLS certificate chain and hostname check for the databroker connection")
	flags.StringVar(&s.tlsOverrideCertificateName, tlsOverrideCertificateName, "",
		"override the certificate name used for the databroker connection")

	flags.StringSliceVar(&s.namespaces, namespaces, nil, "namespaces to watch, or none to watch all namespaces")
	flags.StringVar(&s.sharedSecret, sharedSecret, "",
		"base64-encoded shared secret for signing JWTs")
	flags.BoolVar(&s.debug, debug, false, "enable debug logging")
	if err := flags.MarkHidden("debug"); err != nil {
		return err
	}
	flags.StringVar(&s.updateStatusFromService, updateStatusFromService, "", "update ingress status from given service status (pomerium-proxy)")
	flags.BoolVar(&s.disableCertCheck, disableCertCheck, false, "this flag should only be set if pomerium is configured with insecure_server option")
	flags.StringVar(&s.globalSettings, globalSettings, "",
		fmt.Sprintf("namespace/name to a resource of type %s/Settings", icsv1.GroupVersion.Group))

	v := viper.New()
	var err error
	flags.VisitAll(func(f *pflag.Flag) {
		if err = v.BindEnv(f.Name, envName(f.Name)); err != nil {
			return
		}

		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			if err = flags.Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				return
			}
		}
	})
	return err
}

func (s *serveCmd) exec(*cobra.Command, []string) error {
	s.setupLogger()
	ctx := ctrl.SetupSignalHandler()

	c, err := s.buildController(ctx)
	if err != nil {
		return err
	}

	return s.runController(ctx, c)
}

func (s *serveCmd) getGlobalSettings() (*types.NamespacedName, error) {
	if s.globalSettings == "" {
		return nil, nil
	}

	name, err := util.ParseNamespacedName(s.globalSettings)
	if err != nil {
		return nil, fmt.Errorf("%s=%s: %w", globalSettings, s.globalSettings, err)
	}
	return name, nil
}

func (s *serveCmd) getIngressControllerOptions() ([]ingress.Option, error) {
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

func (s *serveCmd) setupLogger() {
	level := zapcore.InfoLevel
	if s.debug {
		level = zapcore.DebugLevel
	}
	opts := zap.Options{
		Development:     s.debug,
		Level:           level,
		StacktraceLevel: zapcore.DPanicLevel,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
}

func (s *serveCmd) getDataBrokerConnection(ctx context.Context) (*grpc.ClientConn, error) {
	dataBrokerServiceURL, err := url.Parse(s.databrokerServiceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid databroker service url: %w", err)
	}

	sharedSecret, _ := base64.StdEncoding.DecodeString(s.sharedSecret)
	return grpcutil.NewGRPCClientConn(ctx, &grpcutil.Options{
		Address:                 dataBrokerServiceURL,
		ServiceName:             "databroker",
		SignedJWTKey:            sharedSecret,
		RequestTimeout:          defaultGRPCTimeout,
		CA:                      base64.StdEncoding.EncodeToString(s.tlsCA),
		CAFile:                  s.tlsCAFile,
		OverrideCertificateName: s.tlsOverrideCertificateName,
		InsecureSkipVerify:      s.tlsInsecureSkipVerify,
	})
}

func (s *serveCmd) buildController(ctx context.Context) (*leadController, error) {
	opts, err := s.getIngressControllerOptions()
	if err != nil {
		return nil, fmt.Errorf("ingress controller opts: %w", err)
	}

	scheme, err := getScheme()
	if err != nil {
		return nil, fmt.Errorf("get scheme: %w", err)
	}

	conn, err := s.getDataBrokerConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("databroker connection: %w", err)
	}
	client := databroker.NewDataBrokerServiceClient(conn)

	c := &leadController{
		Reconciler: pomerium.WithLock(&pomerium.ConfigReconciler{
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.debug,
		}),
		DataBrokerServiceClient: client,
		MgrOpts: ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: s.metricsAddr,
			Port:               s.webhookPort,
			LeaderElection:     false,
		},
		CtrlOpts:         opts,
		namespaces:       s.namespaces,
		className:        s.className,
		annotationPrefix: s.annotationPrefix,
	}

	c.globalSettings, err = s.getGlobalSettings()
	if err != nil {
		return nil, err
	}

	return c, nil
}

// runController runs controller part of the lease
func (s *serveCmd) runController(ctx context.Context, ct *leadController) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		leaser := databroker.NewLeaser("ingress-controller", leaseDuration, ct)
		return leaser.Run(ctx)
	})
	eg.Go(func() error {
		return s.runHealthz(ctx, healthz.NamedCheck("acquire databroker lease", ct.ReadyzCheck))
	})
	return eg.Wait()
}

// leadController implements a databroker lease holder
type leadController struct {
	pomerium.Reconciler
	databroker.DataBrokerServiceClient
	MgrOpts          ctrl.Options
	CtrlOpts         []ingress.Option
	namespaces       []string
	annotationPrefix string
	className        string
	running          int32
	globalSettings   *types.NamespacedName
}

func (c *leadController) GetDataBrokerServiceClient() databroker.DataBrokerServiceClient {
	return c.DataBrokerServiceClient
}

func (c *leadController) setRunning(running bool) {
	if running {
		atomic.StoreInt32(&c.running, 1)
	} else {
		atomic.StoreInt32(&c.running, 0)
	}
}

func (c *leadController) ReadyzCheck(r *http.Request) error {
	val := atomic.LoadInt32(&c.running)
	if val == 0 {
		return errWaitingForLease
	}
	return nil
}

func (c *leadController) RunLeased(ctx context.Context) error {
	defer c.setRunning(false)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get k8s api config: %w", err)
	}
	mgr, err := ctrl.NewManager(cfg, c.MgrOpts)
	if err != nil {
		return fmt.Errorf("unable to create controller manager: %w", err)
	}

	if err = ingress.NewIngressController(mgr, c.Reconciler, c.CtrlOpts...); err != nil {
		return fmt.Errorf("create ingress controller: %w", err)
	}
	if c.globalSettings != nil {
		if err = settings.NewSettingsController(mgr, c.Reconciler, *c.globalSettings); err != nil {
			return fmt.Errorf("create settings controller: %w", err)
		}
	}

	c.setRunning(true)
	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("running controller: %w", err)
	}
	return nil
}

func (s *serveCmd) runHealthz(ctx context.Context, readyChecks ...healthz.HealthChecker) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := http.NewServeMux()
	healthz.InstallHandler(mux)
	healthz.InstallReadyzHandler(mux, readyChecks...)

	srv := http.Server{
		Addr:    s.probeAddr,
		Handler: mux,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	return srv.ListenAndServe()
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

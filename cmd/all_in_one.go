package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"

	validate "github.com/go-playground/validator/v10"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/types"
	runtime_ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"
	"github.com/pomerium/pomerium/pkg/netutil"

	"github.com/pomerium/ingress-controller/controllers"
	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/pomerium"
	pomerium_ctrl "github.com/pomerium/ingress-controller/pomerium/ctrl"
	"github.com/pomerium/ingress-controller/util"
)

type allCmdOptions struct {
	ingressControllerOpts
	debug bool
	// metricsBindAddress must be externally accessible host:port
	metricsBindAddress string `validate:"required,hostname_port"`
	serverAddr         string `validate:"required,hostname_port"`
	httpRedirectAddr   string `validate:"required,hostname_port"`
}

type allCmdParam struct {
	settings                types.NamespacedName
	ingressOpts             []ingress.Option
	updateStatusFromService string
	dumpConfigDiff          bool

	// bootstrapMetricsAddr for bootstrap configuration controller metrics
	bootstrapMetricsAddr string
	// ingressMetricsAddr for ingress+settings reconciliation controller metrics
	ingressMetricsAddr string

	cfg config.Config
}

type allCmd struct {
	allCmdOptions
	cobra.Command
}

// AllInOneCommand runs embedded pomerium and controls it according to Settings CRD
func AllInOneCommand() (*cobra.Command, error) {
	cmd := allCmd{
		Command: cobra.Command{
			Use:   "all-in-one",
			Short: "run ingress controller together with pomerium in all-in-one mode",
		}}
	cmd.RunE = cmd.exec
	if err := cmd.setupFlags(); err != nil {
		return nil, err
	}
	return &cmd.Command, nil
}

func (s *allCmd) setupFlags() error {
	flags := s.PersistentFlags()
	flags.BoolVar(&s.debug, debug, false, "enable debug logging")
	if err := flags.MarkHidden("debug"); err != nil {
		return err
	}
	flags.StringVar(&s.metricsBindAddress, metricsBindAddress, "", "host:port for aggregate metrics. host is mandatory")
	flags.StringVar(&s.serverAddr, "server-addr", ":8443", "the address the HTTPS server would bind to")
	flags.StringVar(&s.httpRedirectAddr, "http-redirect-addr", ":8080", "the address HTTP redirect would bind to")

	s.ingressControllerOpts.setupFlags(flags)
	return viperWalk(flags)
}

func (s *allCmdOptions) Validate() error {
	return validate.New().Struct(s)
}

func (s *allCmd) exec(*cobra.Command, []string) error {
	if err := s.Validate(); err != nil {
		return err
	}

	setupLogger(s.debug)
	ctx := runtime_ctrl.SetupSignalHandler()

	param, err := s.getParam()
	if err != nil {
		return err
	}

	return param.run(ctx)
}

func (s *allCmdOptions) getParam() (*allCmdParam, error) {
	settings, err := util.ParseNamespacedName(s.GlobalSettings)
	if err != nil {
		return nil, fmt.Errorf("--%s: %w", globalSettings, err)
	}

	if err = s.Validate(); err != nil {
		return nil, fmt.Errorf("args: %w", err)
	}

	opts, err := s.getIngressControllerOptions()
	if err != nil {
		return nil, fmt.Errorf("options: %w", err)
	}

	p := &allCmdParam{
		settings:                *settings,
		ingressOpts:             opts,
		updateStatusFromService: s.UpdateStatusFromService,
		dumpConfigDiff:          s.debug,
	}
	if err := p.makeBootstrapConfig(*s); err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}

	return p, nil
}

func (s *allCmdParam) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)

	runner, err := pomerium_ctrl.NewPomeriumRunner(s.cfg)
	if err != nil {
		return fmt.Errorf("preparing to run pomerium: %w", err)
	}

	eg.Go(func() error { return runner.Run(ctx) })
	eg.Go(func() error { return s.runBootstrapConfigController(ctx, runner) })
	eg.Go(func() error { return s.runConfigControllers(ctx, runner) })

	return eg.Wait()
}

func (s *allCmdParam) makeBootstrapConfig(opt allCmdOptions) error {
	s.cfg.Options = config.NewDefaultOptions()

	s.cfg.Options.Addr = opt.serverAddr
	s.cfg.Options.HTTPRedirectAddr = opt.httpRedirectAddr

	ports, err := netutil.AllocatePorts(7)
	if err != nil {
		return fmt.Errorf("allocating ports: %w", err)
	}

	s.cfg.AllocatePorts(*(*[5]string)(ports[:5]))

	s.bootstrapMetricsAddr = fmt.Sprintf("localhost:%s", ports[5])
	s.ingressMetricsAddr = fmt.Sprintf("localhost:%s", ports[6])

	s.cfg.Options.MetricsAddr = opt.metricsBindAddress

	s.cfg.MetricsScrapeEndpoints = []config.MetricsScrapeEndpoint{
		{
			Name: "bootstrap",
			URL: url.URL{
				Scheme: "http",
				Host:   s.bootstrapMetricsAddr,
				Path:   "/metrics",
			},
			Labels: map[string]string{
				"service": "bootstrap-controller",
			},
		},
		{
			Name: "ingress",
			URL: url.URL{
				Scheme: "http",
				Host:   s.ingressMetricsAddr,
				Path:   "/metrics",
			},
			Labels: map[string]string{
				"service": "ingress-controller",
			},
		},
	}

	return nil
}

// runConfigController runs an integrated Ingress + Settings CRD controller
// TODO: it must be updated in case of configuration change to reconfigure shared_secret
func (s *allCmdParam) runConfigControllers(ctx context.Context, runner *pomerium_ctrl.Runner) error {
	logger := log.FromContext(ctx).WithName("config-controller")
	logger.Info("waiting for config to be available")
	if err := runner.WaitForConfig(ctx); err != nil {
		return fmt.Errorf("waiting for boostrap config: %w", err)
	}
	c, err := s.buildController(ctx, runner.GetConfig())
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}
	logger.Info("received config, starting up controllers")
	return c.Run(ctx)
}

func (s *allCmdParam) getDataBrokerConnection(ctx context.Context, cfg *config.Config) (*grpc.ClientConn, error) {
	sharedSecret, err := base64.StdEncoding.DecodeString(cfg.Options.SharedKey)
	if err != nil {
		return nil, fmt.Errorf("decode shared_secret: %w", err)
	}

	return grpcutil.NewGRPCClientConn(ctx, &grpcutil.Options{
		Address: &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("localhost", cfg.GRPCPort),
		},
		ServiceName:    "databroker",
		SignedJWTKey:   sharedSecret,
		RequestTimeout: defaultGRPCTimeout,
	})
}

func (s *allCmdParam) buildController(ctx context.Context, cfg *config.Config) (*controllers.Controller, error) {
	scheme, err := getScheme()
	if err != nil {
		return nil, fmt.Errorf("get scheme: %w", err)
	}

	conn, err := s.getDataBrokerConnection(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("databroker connection: %w", err)
	}

	client := databroker.NewDataBrokerServiceClient(conn)
	c := &controllers.Controller{
		Reconciler: pomerium.WithLock(&pomerium.DataBrokerReconciler{
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.dumpConfigDiff,
		}),
		DataBrokerServiceClient: client,
		MgrOpts: runtime_ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: s.ingressMetricsAddr,
			Port:               0,
			LeaderElection:     false,
		},
		IngressCtrlOpts: s.ingressOpts,
	}

	return c, nil
}

// runBootstrapConfigController runs a controller that only listens to changes in SettingsCRD
// related to pomerium bootstrap parameters
func (s *allCmdParam) runBootstrapConfigController(ctx context.Context, reconciler pomerium.ConfigReconciler) error {
	scheme, err := getScheme()
	if err != nil {
		return err
	}
	cfg, err := runtime_ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get k8s api config: %w", err)
	}

	mgr, err := runtime_ctrl.NewManager(cfg, runtime_ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: s.bootstrapMetricsAddr,
		Port:               0,
		LeaderElection:     false,
	})
	if err != nil {
		return fmt.Errorf("manager: %w", err)
	}
	if err := settings.NewSettingsController(mgr, reconciler, s.settings); err != nil {
		return fmt.Errorf("settings controller: %w", err)
	}
	return mgr.Start(ctx)
}

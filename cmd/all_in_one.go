package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

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
	debug                           bool
	configControllerShutdownTimeout time.Duration
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

	// configControllerShutdownTimeout is max time to wait for graceful config (ingress & settings) controller shutdown
	// when re-initialization is required. in case it cannot be shut down gracefully, the entire process would exit
	// to be re-started by the kubernetes
	configControllerShutdownTimeout time.Duration

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
	flags.DurationVar(&s.configControllerShutdownTimeout, "config-controller-shutdown", time.Second*30, "timeout waiting for graceful config controller shutdown")
	if err := flags.MarkHidden("config-controller-shutdown"); err != nil {
		return nil
	}
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
	settings, err := util.ParseNamespacedName(s.GlobalSettings, util.WithClusterScope())
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
		settings:                        *settings,
		ingressOpts:                     opts,
		updateStatusFromService:         s.UpdateStatusFromService,
		dumpConfigDiff:                  s.debug,
		configControllerShutdownTimeout: s.configControllerShutdownTimeout,
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
	cfgCtl := util.NewRestartOnChange[*config.Config]()
	runner, err := pomerium_ctrl.NewPomeriumRunner(s.cfg, cfgCtl.OnConfigUpdated)
	if err != nil {
		return fmt.Errorf("preparing to run pomerium: %w", err)
	}

	eg.Go(func() error { return runner.Run(ctx) })
	eg.Go(func() error { return s.runBootstrapConfigController(ctx, runner) })
	eg.Go(func() error {
		return cfgCtl.Run(log.IntoContext(ctx, log.FromContext(ctx).WithName("config_restarter")),
			isBootstrapEqual,
			s.runConfigControllers,
			s.configControllerShutdownTimeout)
	})

	return eg.Wait()
}

func (s *allCmdParam) makeBootstrapConfig(opt allCmdOptions) error {
	s.cfg.Options = config.NewDefaultOptions()

	s.cfg.Options.Addr = opt.serverAddr
	s.cfg.Options.HTTPRedirectAddr = opt.httpRedirectAddr

	ports, err := netutil.AllocatePorts(8)
	if err != nil {
		return fmt.Errorf("allocating ports: %w", err)
	}

	s.cfg.AllocatePorts(*(*[6]string)(ports[:6]))

	s.bootstrapMetricsAddr = fmt.Sprintf("localhost:%s", ports[6])
	s.ingressMetricsAddr = fmt.Sprintf("localhost:%s", ports[7])

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
func (s *allCmdParam) runConfigControllers(ctx context.Context, cfg *config.Config) error {
	c, err := s.buildController(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	return c.Run(ctx)
}

func getDataBrokerConnection(ctx context.Context, cfg *config.Config) (*grpc.ClientConn, error) {
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

	conn, err := getDataBrokerConnection(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("databroker connection: %w", err)
	}

	client := databroker.NewDataBrokerServiceClient(conn)
	c := &controllers.Controller{
		IngressReconciler: &pomerium.DataBrokerReconciler{
			ConfigID:                pomerium.IngressControllerConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.dumpConfigDiff,
			RemoveUnreferencedCerts: true,
		},
		ConfigReconciler: &pomerium.DataBrokerReconciler{
			ConfigID:                pomerium.SharedSettingsConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.dumpConfigDiff,
			RemoveUnreferencedCerts: false,
		},
		DataBrokerServiceClient: client,
		MgrOpts: runtime_ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: s.ingressMetricsAddr,
			Port:               0,
			LeaderElection:     false,
		},
		IngressCtrlOpts: s.ingressOpts,
		GlobalSettings:  &s.settings,
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
	name := "bootstrap"
	if host, err := os.Hostname(); err == nil {
		name = fmt.Sprintf("%s pod/%s", name, host)
	}
	if err := settings.NewSettingsController(mgr, reconciler, s.settings, name, false); err != nil {
		return fmt.Errorf("settings controller: %w", err)
	}
	return mgr.Start(ctx)
}

// isBootstrapEqual returns true if two configs are equal for the purpose of bootstrapping configuration controllers
// right now we only care about sharedkey as it is used for databroker
// and we do not update port allocations
func isBootstrapEqual(prev, next *config.Config) bool {
	if prev == nil {
		return false
	}
	return prev.Options.SharedKey == next.Options.SharedKey
}

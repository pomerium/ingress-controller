package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	validate "github.com/go-playground/validator/v10"
	"github.com/spf13/cobra"
	"github.com/volatiletech/null/v9"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	runtime_ctrl "sigs.k8s.io/controller-runtime"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/pomerium/ingress-controller/controllers"
	"github.com/pomerium/ingress-controller/controllers/gateway"
	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/pomerium"
	pomerium_ctrl "github.com/pomerium/ingress-controller/pomerium/ctrl"
	"github.com/pomerium/ingress-controller/util"
	health_ctrl "github.com/pomerium/ingress-controller/util/health"
	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/health"
	"github.com/pomerium/pomerium/pkg/netutil"
	"github.com/pomerium/pomerium/pkg/telemetry/trace"
)

type allCmdOptions struct {
	ingressControllerOpts
	debug                           bool
	debugDumpConfigDiff             bool
	debugPomerium                   bool
	debugEnvoy                      bool
	adminBindAddr                   string
	configControllerShutdownTimeout time.Duration
	// healthProbeBindAddress must be externally accessible host:port
	healthProbeBindAddress string
	// metricsBindAddress must be externally accessible host:port
	metricsBindAddress string `validate:"required,hostname_port"`
	serverAddr         string `validate:"required,hostname_port"`
	sshAddr            string
	httpRedirectAddr   string   `validate:"required,hostname_port"`
	deriveTLS          string   `validate:"required,hostname"`
	grpcAddr           string   `validate:"required,hostname_port"`
	services           []string `validate:"dive,oneof=all authenticate authorize databroker proxy"`

	DataBrokerOptions dataBrokerOptions
}

type allCmdParam struct {
	settings                types.NamespacedName
	ingressOpts             []ingress.Option
	gatewayConfig           *gateway.ControllerConfig
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
		},
	}
	cmd.RunE = cmd.exec
	if err := cmd.setupFlags(); err != nil {
		return nil, err
	}
	return &cmd.Command, nil
}

// the below flags are not intended to be used by end users, but rather for development and debugging purposes
// setting them to hidden to avoid confusion, as enabling them may cause sensitive information to be logged or exposed
const (
	debug                    = "debug"
	debugPomerium            = "debug-pomerium"
	debugEnvoy               = "debug-envoy"
	debugAdminBindAddr       = "debug-admin-addr"
	debugDumpConfigDiff      = "debug-dump-config-diff"
	configControllerShutdown = "config-controller-shutdown"
)

var hidden = []string{
	debugPomerium,
	debugEnvoy,
	debugAdminBindAddr,
	debugDumpConfigDiff,
}

func (s *allCmd) setupFlags() error {
	flags := s.PersistentFlags()
	flags.BoolVar(&s.debug, debug, false, "enable debug logging")
	flags.BoolVar(&s.debugDumpConfigDiff, debugDumpConfigDiff, false, "development dump of config diff, don't use in production")
	flags.BoolVar(&s.debugPomerium, debugPomerium, false, "enable debug logging for pomerium")
	flags.BoolVar(&s.debugEnvoy, debugEnvoy, false, "enable debug logging for envoy")
	flags.StringVar(&s.metricsBindAddress, metricsBindAddress, "", "host:port for aggregate metrics. host is mandatory")
	flags.StringVar(&s.healthProbeBindAddress, healthProbeBindAddress, "127.0.0.1:28080", "host:port for http health probes")
	flags.StringVar(&s.adminBindAddr, debugAdminBindAddr, "", "host:port for admin server")
	flags.StringVar(&s.serverAddr, "server-addr", ":8443", "the address the HTTPS server would bind to")
	flags.StringVar(&s.grpcAddr, "grpc-addr", ":5443", "the address the gRPC server would bind to")
	flags.StringVar(&s.sshAddr, "ssh-addr", "", "the address the SSH server would bind to")
	flags.StringVar(&s.httpRedirectAddr, "http-redirect-addr", ":8080", "the address HTTP redirect would bind to")
	flags.StringVar(&s.deriveTLS, "databroker-auto-tls", "", "enable auto TLS and generate server certificate for the domain")
	flags.DurationVar(&s.configControllerShutdownTimeout, configControllerShutdown, time.Second*30, "timeout waiting for graceful config controller shutdown")
	flags.StringSliceVar(&s.services, "services", []string{"all"}, "the pomerium services to run")

	for _, flag := range hidden {
		if err := s.PersistentFlags().MarkHidden(flag); err != nil {
			return fmt.Errorf("failed to mark %s flag: %w", flag, err)
		}
	}

	s.ingressControllerOpts.setupFlags(flags)
	s.DataBrokerOptions.setupFlags(flags)
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

	gatewayConfig, err := s.getGatewayControllerConfig()
	if err != nil {
		return nil, fmt.Errorf("options: %w", err)
	}

	p := &allCmdParam{
		settings:                        *settings,
		ingressOpts:                     opts,
		gatewayConfig:                   gatewayConfig,
		updateStatusFromService:         s.UpdateStatusFromService,
		dumpConfigDiff:                  s.debugDumpConfigDiff,
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

	ctx = trace.NewContext(ctx, trace.NewSyncClient(nil))
	ctx = health.Context(
		ctx,
		health_ctrl.SettingsBootstrapReconciler,
	)

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
	eg.Go(func() error {
		<-ctx.Done()
		if err := trace.ShutdownContext(ctx); err != nil {
			log.FromContext(ctx).Error(err, "failed to shutdown trace context")
		}
		return nil
	})

	return eg.Wait()
}

func (s *allCmdParam) makeBootstrapConfig(opt allCmdOptions) error {
	s.cfg.Options = config.NewDefaultOptions()

	s.cfg.Options.Services = strings.Join(opt.services, ",")
	s.cfg.Options.Addr = opt.serverAddr
	s.cfg.Options.GRPCAddr = opt.grpcAddr
	s.cfg.Options.HTTPRedirectAddr = opt.httpRedirectAddr

	ports, err := netutil.AllocatePorts(8)
	if err != nil {
		return fmt.Errorf("allocating ports: %w", err)
	}

	s.cfg.AllocatePorts(*(*[6]string)(ports[:6]))

	if opt.deriveTLS != "" {
		s.cfg.Options.DeriveInternalDomainCert = &opt.deriveTLS
		s.cfg.Options.GRPCInsecure = proto.Bool(false)
	}

	s.bootstrapMetricsAddr = fmt.Sprintf("localhost:%s", ports[6])
	s.ingressMetricsAddr = fmt.Sprintf("localhost:%s", ports[7])

	s.cfg.Options.MetricsAddr = opt.metricsBindAddress
	s.cfg.Options.HealthCheckAddr = opt.healthProbeBindAddress

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

	if opt.debugPomerium {
		s.cfg.Options.LogLevel = "debug"
	} else {
		s.cfg.Options.LogLevel = "info"
	}
	if opt.debugEnvoy {
		s.cfg.Options.ProxyLogLevel = "debug"
		s.cfg.Options.LogLevel = "debug"
	}
	s.cfg.Options.EnvoyAdminAddress = opt.adminBindAddr
	s.cfg.Options.HTTP3AdvertisePort = null.NewUint32(443, true)
	s.cfg.Options.SSHAddr = opt.sshAddr

	opt.DataBrokerOptions.apply(&s.cfg)

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
		GatewayReconciler: &pomerium.DataBrokerReconciler{
			ConfigID:                pomerium.GatewayControllerConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.dumpConfigDiff,
			RemoveUnreferencedCerts: false,
		},
		DataBrokerServiceClient: client,
		MgrOpts: runtime_ctrl.Options{
			Scheme: scheme,
			Metrics: metricsserver.Options{
				BindAddress: s.ingressMetricsAddr,
			},
			LeaderElection: false,
			Controller: controllerconfig.Controller{
				SkipNameValidation: ptr.To(true),
			},
		},
		IngressCtrlOpts:         s.ingressOpts,
		GlobalSettings:          &s.settings,
		GatewayControllerConfig: s.gatewayConfig,
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
		Scheme:         scheme,
		Metrics:        metricsserver.Options{BindAddress: s.bootstrapMetricsAddr},
		LeaderElection: false,
	})
	if err != nil {
		return fmt.Errorf("manager: %w", err)
	}
	name := settings.ControllerNameBootstrap
	if host, err := os.Hostname(); err == nil {
		name = fmt.Sprintf("%s pod/%s", name, host)
	}
	if err := settings.NewSettingsController(mgr, reconciler, s.settings, name, false, health_ctrl.SettingsBootstrapReconciler); err != nil {
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

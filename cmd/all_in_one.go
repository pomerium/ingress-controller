package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/types"
	runtime_ctrl "sigs.k8s.io/controller-runtime"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"

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
}

type allCmdParam struct {
	settings                types.NamespacedName
	ingressOpts             []ingress.Option
	updateStatusFromService string
	dumpConfigDiff          bool
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
	s.ingressControllerOpts.setupFlags(flags)
	return viperWalk(flags)
}

func (s *allCmd) exec(*cobra.Command, []string) error {
	setupLogger(s.debug)
	ctx := runtime_ctrl.SetupSignalHandler()

	param, err := s.getParam()
	if err != nil {
		return err
	}

	return param.run(ctx)
}

func (s *allCmdOptions) getParam() (*allCmdParam, error) {
	settings, err := util.ParseNamespacedName(s.globalSettings)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", globalSettings, err)
	}

	if err = s.Validate(); err != nil {
		return nil, fmt.Errorf("args: %w", err)
	}

	opts, err := s.getIngressControllerOptions()
	if err != nil {
		return nil, fmt.Errorf("options: %w", err)
	}

	return &allCmdParam{
		settings:                *settings,
		ingressOpts:             opts,
		updateStatusFromService: s.updateStatusFromService,
		dumpConfigDiff:          s.debug,
	}, nil
}

func (s *allCmdParam) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)

	runner, err := pomerium_ctrl.NewPomeriumRunner()
	if err != nil {
		return fmt.Errorf("preparing to run pomerium: %w", err)
	}

	eg.Go(func() error { return runner.Run(ctx) })
	eg.Go(func() error { return s.runBootstrapConfigController(ctx, runner) })
	eg.Go(func() error { return s.runConfigControllers(ctx, runner) })

	return eg.Wait()
}

// runConfigController runs an integrated Ingress + Settings CRD controller
// TODO: it must be updated in case of configuration change to reconfigure shared_secret
func (s *allCmdParam) runConfigControllers(ctx context.Context, runner *pomerium_ctrl.Runner) error {
	if err := runner.WaitForConfig(ctx); err != nil {
		return fmt.Errorf("waiting for boostrap config: %w", err)
	}
	c, err := s.buildController(ctx, runner.GetConfig())
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}
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
			MetricsBindAddress: "0",
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
		MetricsBindAddress: "0",
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
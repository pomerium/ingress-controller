package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apiserver/pkg/server/healthz"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"

	"github.com/pomerium/ingress-controller/controllers"
	"github.com/pomerium/ingress-controller/pomerium"
)

type controllerCmd struct {
	ingressControllerOpts

	metricsAddr string
	probeAddr   string

	databrokerServiceURL       string
	tlsCAFile                  string
	tlsCA                      []byte
	tlsInsecureSkipVerify      bool
	tlsOverrideCertificateName string

	sharedSecret string

	debug bool

	cobra.Command
	pomerium.IngressReconciler
}

// ControllerCommand creates command to run ingress controller
func ControllerCommand() (*cobra.Command, error) {
	cmd := controllerCmd{
		Command: cobra.Command{
			Use:   "controller",
			Short: "runs just the ingress controller",
		}}
	cmd.RunE = cmd.exec
	if err := cmd.setupFlags(); err != nil {
		return nil, err
	}
	return &cmd.Command, nil
}

const (
	databrokerServiceURL       = "databroker-service-url"
	databrokerTLSCAFile        = "databroker-tls-ca-file"
	databrokerTLSCA            = "databroker-tls-ca"
	tlsInsecureSkipVerify      = "databroker-tls-insecure-skip-verify"
	tlsOverrideCertificateName = "databroker-tls-override-certificate-name"
)

func (s *controllerCmd) setupFlags() error {
	flags := s.PersistentFlags()
	flags.StringVar(&s.metricsAddr, metricsBindAddress, ":9090", "The address the metric endpoint binds to.")
	flags.StringVar(&s.probeAddr, healthProbeBindAddress, ":8081", "The address the probe endpoint binds to.")
	flags.StringVar(&s.databrokerServiceURL, databrokerServiceURL, "http://localhost:5443",
		"the databroker service url")
	flags.StringVar(&s.tlsCAFile, databrokerTLSCAFile, "", "tls CA file path")
	flags.BytesBase64Var(&s.tlsCA, databrokerTLSCA, nil, "base64 encoded tls CA")
	flags.BoolVar(&s.tlsInsecureSkipVerify, tlsInsecureSkipVerify, false,
		"disable remote hosts TLS certificate chain and hostname check for the databroker connection")
	flags.StringVar(&s.tlsOverrideCertificateName, tlsOverrideCertificateName, "",
		"override the certificate name used for the databroker connection")

	flags.StringVar(&s.sharedSecret, sharedSecret, "",
		"base64-encoded shared secret for signing JWTs")
	flags.BoolVar(&s.debug, debug, false, "enable debug logging")
	if err := flags.MarkHidden("debug"); err != nil {
		return err
	}

	s.ingressControllerOpts.setupFlags(flags)
	return viperWalk(flags)
}

func (s *controllerCmd) exec(*cobra.Command, []string) error {
	setupLogger(s.debug)
	ctx := ctrl.SetupSignalHandler()

	c, err := s.buildController(ctx)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return runHealthz(ctx, s.probeAddr, healthz.NamedCheck("acquire databroker lease", c.ReadyzCheck))
	})
	eg.Go(func() error { return c.Run(ctx) })

	return eg.Wait()
}

func (s *controllerCmd) getDataBrokerConnection(ctx context.Context) (*grpc.ClientConn, error) {
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

func (s *controllerCmd) buildController(ctx context.Context) (*controllers.Controller, error) {
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

	c := &controllers.Controller{
		IngressReconciler: &pomerium.DataBrokerReconciler{
			ConfigID:                pomerium.IngressControllerConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.debug,
			RemoveUnreferencedCerts: true,
		},
		ConfigReconciler: &pomerium.DataBrokerReconciler{
			ConfigID:                pomerium.SharedSettingsConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.debug,
			RemoveUnreferencedCerts: false,
		},
		GatewayReconciler: &pomerium.DataBrokerReconciler{
			ConfigID:                pomerium.GatewayControllerConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     s.debug,
			RemoveUnreferencedCerts: false,
		},
		DataBrokerServiceClient: client,
		MgrOpts: ctrl.Options{
			Scheme:         scheme,
			Metrics:        metricsserver.Options{BindAddress: s.metricsAddr},
			LeaderElection: false,
		},
		IngressCtrlOpts: opts,
	}

	c.GlobalSettings, err = s.getGlobalSettings()
	if err != nil {
		return nil, err
	}

	return c, nil
}

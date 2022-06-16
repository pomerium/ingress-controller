package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/controllers"
	"github.com/pomerium/ingress-controller/controllers/ingress"
	"github.com/pomerium/ingress-controller/pomerium"
	"github.com/pomerium/ingress-controller/util"
)

type controllerCmd struct {
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

func (s *controllerCmd) setupFlags() error {
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

func (s *controllerCmd) exec(*cobra.Command, []string) error {
	setupLogger(s.debug)
	ctx := ctrl.SetupSignalHandler()

	c, err := s.buildController(ctx)
	if err != nil {
		return err
	}

	return c.Run(ctx)
}

func (s *controllerCmd) getGlobalSettings() (*types.NamespacedName, error) {
	if s.globalSettings == "" {
		return nil, nil
	}

	name, err := util.ParseNamespacedName(s.globalSettings)
	if err != nil {
		return nil, fmt.Errorf("%s=%s: %w", globalSettings, s.globalSettings, err)
	}
	return name, nil
}

func (s *controllerCmd) getIngressControllerOptions() ([]ingress.Option, error) {
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
		Reconciler: pomerium.WithLock(&pomerium.DataBrokerReconciler{
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
		IngressCtrlOpts: opts,
	}

	c.GlobalSettings, err = s.getGlobalSettings()
	if err != nil {
		return nil, err
	}

	return c, nil
}

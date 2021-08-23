package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	pomeriumgrpc "github.com/pomerium/pomerium/pkg/grpc"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"

	"github.com/pomerium/ingress-controller/controllers"
	"github.com/pomerium/ingress-controller/pomerium"
)

const (
	defaultGRPCTimeout = time.Minute
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

type serveCmd struct {
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string

	databrokerServiceURL string
	sharedSecret         string
	tlsCA                string

	debug bool

	cobra.Command
	manager.Manager
	controllers.PomeriumReconciler
}

func ServeCommand() *cobra.Command {
	cmd := serveCmd{
		Command: cobra.Command{
			Use:   "serve",
			Short: "run ingress controller",
		}}
	cmd.RunE = cmd.exec
	cmd.setupFlags()
	return &cmd.Command
}

func (s *serveCmd) setupFlags() {
	flags := s.PersistentFlags()
	flags.StringVar(&s.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flags.StringVar(&s.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flags.BoolVar(&s.enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flags.StringVar(&s.databrokerServiceURL, "databroker-service-url", "http://localhost:5443",
		"the databroker service url")
	flags.StringVar(&s.sharedSecret, "shared-secret", "",
		"base64-encoded shared secret for signing JWTs")
	flags.BoolVar(&s.debug, "debug", true, "enable debug logging")
}

func (c *serveCmd) exec(*cobra.Command, []string) error {
	c.setupLogger()

	if err := c.setupConfigReconciler(); err != nil {
		return err
	}
	if err := c.setupController(); err != nil {
		return err
	}
	return c.Manager.Start(c.Context())
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

func (s *serveCmd) setupController() error {
	mgr, err := controllers.NewIngressController(ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     s.metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: s.probeAddr,
		LeaderElection:         s.enableLeaderElection,
		LeaderElectionID:       "996e99b1.pomerium.io",
	}, s.PomeriumReconciler)
	if err != nil {
		return err
	}
	s.Manager = mgr
	return nil
}

func (s *serveCmd) setupConfigReconciler() error {
	dbc, err := s.getDataBrokerConnection(s.Context())
	if err != nil {
		return fmt.Errorf("databroker connection: %w", err)
	}
	s.PomeriumReconciler = &pomerium.ConfigReconciler{
		DataBrokerServiceClient: databroker.NewDataBrokerServiceClient(dbc),
	}
	return nil
}

func (s *serveCmd) getDataBrokerConnection(ctx context.Context) (*grpc.ClientConn, error) {
	dataBrokerServiceURL, err := url.Parse(s.databrokerServiceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid databroker service url: %w", err)
	}

	sharedSecret, _ := base64.StdEncoding.DecodeString(s.sharedSecret)
	return pomeriumgrpc.NewGRPCClientConn(ctx, &pomeriumgrpc.Options{
		Addrs:          []*url.URL{dataBrokerServiceURL},
		WithInsecure:   dataBrokerServiceURL.Scheme != "https",
		ServiceName:    "databroker",
		SignedJWTKey:   sharedSecret,
		RequestTimeout: defaultGRPCTimeout,
	})
}

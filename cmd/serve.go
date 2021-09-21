// Package cmd implements top level commands
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

	"github.com/pomerium/pomerium/pkg/grpc/databroker"

	"github.com/pomerium/ingress-controller/controllers"
	"github.com/pomerium/ingress-controller/pomerium"
)

const (
	defaultGRPCTimeout = time.Minute
	leaseDuration      = time.Second * 30
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

type serveCmd struct {
	metricsAddr      string
	webhookPort      int
	probeAddr        string
	className        string
	annotationPrefix string
	namespaces       []string

	databrokerServiceURL string
	sharedSecret         string

	debug bool

	cobra.Command
	controllers.PomeriumReconciler
}

// ServeCommand creates command to run ingress controller
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
	flags.IntVar(&s.webhookPort, "webhook-port", 9443, "webhook port")
	flags.StringVar(&s.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flags.StringVar(&s.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flags.StringVar(&s.className, "name", controllers.DefaultClassControllerName, "IngressClass controller name")
	flags.StringVar(&s.annotationPrefix, "prefix", controllers.DefaultAnnotationPrefix, "Ingress annotation prefix")
	flags.StringVar(&s.databrokerServiceURL, "databroker-service-url", "http://localhost:5443",
		"the databroker service url")
	flags.StringArrayVar(&s.namespaces, "namespaces", nil, "namespaces to watch, or none to watch all namespaces")
	flags.StringVar(&s.sharedSecret, "shared-secret", "",
		"base64-encoded shared secret for signing JWTs")
	flags.BoolVar(&s.debug, "debug", true, "enable debug logging")
}

func (s *serveCmd) exec(*cobra.Command, []string) error {
	s.setupLogger()
	ctx := ctrl.SetupSignalHandler()
	dbc, err := s.getDataBrokerConnection(ctx)
	if err != nil {
		return fmt.Errorf("databroker connection: %w", err)
	}

	return s.runController(ctx,
		databroker.NewDataBrokerServiceClient(dbc),
		ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     s.metricsAddr,
			Port:                   s.webhookPort,
			HealthProbeBindAddress: s.probeAddr,
			LeaderElection:         false,
		})
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
	return NewGRPCClientConn(ctx, &Options{
		Address:        dataBrokerServiceURL,
		WithInsecure:   dataBrokerServiceURL.Scheme != "https",
		ServiceName:    "databroker",
		SignedJWTKey:   sharedSecret,
		RequestTimeout: defaultGRPCTimeout,
	})
}

type leadController struct {
	controllers.PomeriumReconciler
	databroker.DataBrokerServiceClient
	ctrl.Options
	namespaces       []string
	annotationPrefix string
	className        string
}

func (c *leadController) GetDataBrokerServiceClient() databroker.DataBrokerServiceClient {
	return c.DataBrokerServiceClient
}

func (c *leadController) RunLeased(ctx context.Context) error {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get k8s api config: %w", err)
	}
	mgr, err := controllers.NewIngressController(ctx, cfg, c.Options, c.PomeriumReconciler,
		controllers.WithNamespaces(c.namespaces),
		controllers.WithAnnotationPrefix(c.annotationPrefix),
		controllers.WithControllerName(c.className),
	)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}
	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("running controller: %w", err)
	}
	return nil
}

func (s *serveCmd) runController(ctx context.Context, client databroker.DataBrokerServiceClient, opts ctrl.Options) error {
	c := &leadController{
		PomeriumReconciler:      &pomerium.ConfigReconciler{DataBrokerServiceClient: client},
		DataBrokerServiceClient: client,
		Options:                 opts,
		namespaces:              s.namespaces,
		className:               s.className,
		annotationPrefix:        s.annotationPrefix,
	}
	leaser := databroker.NewLeaser("ingress-controller", leaseDuration, c)
	return leaser.Run(ctx)
}

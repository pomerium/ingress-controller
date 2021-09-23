// Package cmd implements top level commands
package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	databrokerServiceURL       string
	tlsCAFile                  string
	tlsCA                      []byte
	tlsInsecureSkipVerify      bool
	tlsOverrideCertificateName string

	sharedSecret string

	updateStatusFromService string

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
	flags.StringVar(&s.tlsCAFile, "databroker-tls-ca-file", "", "tls CA file path")
	flags.BytesBase64Var(&s.tlsCA, "databroker-tls-ca", nil, "base64 encoded tls CA")
	flags.BoolVar(&s.tlsInsecureSkipVerify, "databroker-tls-insecure-skip-verify", false,
		"disable remote hosts TLS certificate chain and hostname check for the databroker connection")
	flags.StringVar(&s.tlsOverrideCertificateName, "databroker-tls-override-certificate-name", "",
		"override the certificate name used for the databroker connection")

	flags.StringArrayVar(&s.namespaces, "namespaces", nil, "namespaces to watch, or none to watch all namespaces")
	flags.StringVar(&s.sharedSecret, "shared-secret", "",
		"base64-encoded shared secret for signing JWTs")
	flags.BoolVar(&s.debug, "debug", true, "enable debug logging")
	flags.MarkHidden("debug")
	flags.StringVar(&s.updateStatusFromService, "update-status-from-service", "", "update ingress status from given service status (pomerium-proxy)")
}

func (s *serveCmd) exec(*cobra.Command, []string) error {
	s.setupLogger()
	ctx := ctrl.SetupSignalHandler()
	dbc, err := s.getDataBrokerConnection(ctx)
	if err != nil {
		return fmt.Errorf("databroker connection: %w", err)
	}

	opts, err := s.getOptions()
	if err != nil {
		return err
	}

	return s.runController(ctx,
		databroker.NewDataBrokerServiceClient(dbc),
		ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     s.metricsAddr,
			Port:                   s.webhookPort,
			HealthProbeBindAddress: s.probeAddr,
			LeaderElection:         false,
		}, opts...)
}

func (s *serveCmd) getOptions() ([]controllers.Option, error) {
	opts := []controllers.Option{
		controllers.WithNamespaces(s.namespaces),
		controllers.WithAnnotationPrefix(s.annotationPrefix),
		controllers.WithControllerName(s.className),
	}
	if s.updateStatusFromService != "" {
		parts := strings.Split(s.updateStatusFromService, "/")
		if len(parts) != 2 {
			return nil, errors.New("service name must be in namespace/name format")
		}
		opts = append(opts,
			controllers.WithUpdateIngressStatusFromService(types.NamespacedName{Namespace: parts[0], Name: parts[1]}))
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
	return NewGRPCClientConn(ctx, &Options{
		Address:                 dataBrokerServiceURL,
		WithInsecure:            dataBrokerServiceURL.Scheme != "https" || s.tlsInsecureSkipVerify,
		ServiceName:             "databroker",
		SignedJWTKey:            sharedSecret,
		RequestTimeout:          defaultGRPCTimeout,
		CA:                      base64.StdEncoding.EncodeToString(s.tlsCA),
		CAFile:                  s.tlsCAFile,
		OverrideCertificateName: s.tlsOverrideCertificateName,
	})
}

type leadController struct {
	controllers.PomeriumReconciler
	databroker.DataBrokerServiceClient
	MgrOpts          ctrl.Options
	CtrlOpts         []controllers.Option
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
	mgr, err := controllers.NewIngressController(ctx, cfg, c.MgrOpts, c.PomeriumReconciler, c.CtrlOpts...)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}
	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("running controller: %w", err)
	}
	return nil
}

func (s *serveCmd) runController(ctx context.Context, client databroker.DataBrokerServiceClient, opts ctrl.Options, cOpts ...controllers.Option) error {
	c := &leadController{
		PomeriumReconciler:      &pomerium.ConfigReconciler{DataBrokerServiceClient: client, DebugDumpConfigDiff: s.debug},
		DataBrokerServiceClient: client,
		MgrOpts:                 opts,
		CtrlOpts:                cOpts,
		namespaces:              s.namespaces,
		className:               s.className,
		annotationPrefix:        s.annotationPrefix,
	}
	leaser := databroker.NewLeaser("ingress-controller", leaseDuration, c)
	return leaser.Run(ctx)
}

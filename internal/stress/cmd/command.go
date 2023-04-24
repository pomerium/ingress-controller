// Package cmd provides the stress test command
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/pomerium/ingress-controller/internal/stress"
)

// Command returns the stress test command
func Command() (*cobra.Command, error) {
	return &cobra.Command{
		Use:   "stress-test",
		Short: "stress test the ingress controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			setupLogger()
			return run(withInterruptSignal(context.Background()))
		},
	}, nil
}

func setupLogger() {
	logger := zerolog.New(os.Stdout)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.DefaultContextLogger = &logger
}

func testConfigFromEnv() (*stress.IngressLoadTestConfig, error) {
	svcName := os.Getenv("SERVICE_NAME")
	if svcName == "" {
		return nil, fmt.Errorf("SERVICE_NAME environment variable not set")
	}

	svcNamespace := os.Getenv("SERVICE_NAMESPACE")
	if svcNamespace == "" {
		return nil, fmt.Errorf("SERVICE_NAMESPACE environment variable not set")
	}

	servicePortNames := strings.Split(os.Getenv("SERVICE_PORT_NAMES"), ",")
	if len(servicePortNames) != 2 {
		return nil, fmt.Errorf("SERVICE_PORT_NAMES environment variable must have exacly two comma separated values")
	}

	domain := os.Getenv("INGRESS_DOMAIN")
	if domain == "" {
		return nil, fmt.Errorf("INGRESS_DOMAIN environment variable not set")
	}

	ingressCount, err := strconv.Atoi(os.Getenv("INGRESS_COUNT"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse INGRESS_COUNT: %w", err)
	}

	ingressClass := os.Getenv("INGRESS_CLASS")
	if ingressClass == "" {
		return nil, fmt.Errorf("INGRESS_CLASS environment variable not set")
	}

	readinessTimeout, err := time.ParseDuration(os.Getenv("READINESS_TIMEOUT"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse READINESS_TIMEOUT: %w", err)
	}

	return &stress.IngressLoadTestConfig{
		ReadinessTimeout: readinessTimeout,
		IngressClass:     ingressClass,
		IngressCount:     ingressCount,
		ServicePortNames: servicePortNames,
		ServiceName:      types.NamespacedName{Name: svcName, Namespace: svcNamespace},
		Domain:           domain,
	}, nil
}

func getKubeClient() (*kubernetes.Clientset, error) {
	var kubeconfig *rest.Config
	kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")
	if _, err := os.Stat(kubeconfigPath); err == nil {
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig from %s: %w", kubeconfigPath, err)
		}
	} else {
		kubeconfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in cluster kubeconfig: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return client, nil
}

func run(ctx context.Context) error {
	cfg, err := testConfigFromEnv()
	if err != nil {
		return err
	}

	client, err := getKubeClient()
	if err != nil {
		return err
	}

	cfg.Client = client
	srv := stress.IngressLoadTest{IngressLoadTestConfig: *cfg}
	err = srv.Run(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error running stress test...")
	}
	return err
}

func withInterruptSignal(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		cancel()
	}()
	return ctx
}

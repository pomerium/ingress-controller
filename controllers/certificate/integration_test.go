package certificate_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	certmanager_api "github.com/cert-manager/cert-manager/pkg/api"
	certmanager_v1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/types/known/timestamppb"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pomerium_ingress_v1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/controllers/certificate"
	"github.com/pomerium/ingress-controller/internal/testutil"
	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
	databrokerpb "github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"
)

const (
	testNamespace          = "pomerium"
	testGlobalSettingsName = "pomerium-settings"
	testClusterIssuer      = "test-issuer"
)

var (
	_ suite.SetupAllSuite     = &ControllerTestSuite{}
	_ suite.TearDownAllSuite  = &ControllerTestSuite{}
	_ suite.SetupTestSuite    = &ControllerTestSuite{}
	_ suite.TearDownTestSuite = &ControllerTestSuite{}
)

type ControllerTestSuite struct {
	suite.Suite
	client.Client
	*envtest.Environment
}

func (s *ControllerTestSuite) SetupSuite() {
	scheme := runtime.NewScheme()
	s.NoError(clientgoscheme.AddToScheme(scheme))
	s.NoError(pomerium_ingress_v1.AddToScheme(scheme))
	s.NoError(certmanager_api.AddToScheme(scheme))

	s.Environment = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			certManagerCRDPath(s.T()),
		},
		ErrorIfCRDPathMissing: true,
		Scheme:                scheme,
		UseExistingCluster:    new(false),
	}
	cfg, err := s.Environment.Start()
	s.NoError(err)
	require.NotNil(s.T(), cfg)

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	s.NoError(err)
	require.NotNil(s.T(), k8sClient)
	s.Client = k8sClient

	s.NoError(s.Client.Create(context.Background(), &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{Name: testNamespace},
	}))
}

func (s *ControllerTestSuite) SetupTest() {
	log.SetLogger(zapr.NewLogger(zaptest.NewLogger(s.T())))
}

func (s *ControllerTestSuite) TearDownTest() {
	s.deleteAll()
}

func (s *ControllerTestSuite) TearDownSuite() {
	s.NoError(s.Environment.Stop())
}

func (s *ControllerTestSuite) deleteAll() {
	ctx := context.Background()

	certs := new(certmanager_v1.CertificateList)
	s.NoError(s.Client.List(ctx, certs))
	for i := range certs.Items {
		s.NoError(s.Client.Delete(ctx, &certs.Items[i]))
	}

	secrets := new(core_v1.SecretList)
	s.NoError(s.Client.List(ctx, secrets, client.InNamespace(testNamespace)))
	for i := range secrets.Items {
		s.NoError(s.Client.Delete(ctx, &secrets.Items[i]))
	}

	settings := new(pomerium_ingress_v1.PomeriumList)
	s.NoError(s.Client.List(ctx, settings))
	for i := range settings.Items {
		s.NoError(s.Client.Delete(ctx, &settings.Items[i]))
	}
}

func (s *ControllerTestSuite) startController(ctx context.Context, client databrokerpb.DataBrokerServiceClient) {
	mgr, err := controllerruntime.NewManager(s.Environment.Config, controllerruntime.Options{
		Scheme:                 s.Environment.Scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller: controllerconfig.Controller{
			SkipNameValidation: new(true),
		},
	})
	s.NoError(err)
	s.NoError(certificate.NewCertificateController(
		mgr,
		client,
		certificate.WithGlobalSettingsName(types.NamespacedName{Name: testGlobalSettingsName}),
		certificate.WithNamespace(testNamespace),
	))

	go func() {
		if err := mgr.Start(ctx); err != nil && ctx.Err() == nil {
			s.T().Errorf("manager exited with error: %v", err)
		}
	}()
}

func (s *ControllerTestSuite) putRoute(ctx context.Context, client databrokerpb.DataBrokerServiceClient, id, from string) {
	s.T().Helper()
	record := databrokerpb.NewRecord(&configpb.Route{Id: new(id), From: from})
	_, err := client.Put(ctx, &databrokerpb.PutRequest{
		Records: []*databrokerpb.Record{record},
	})
	require.NoError(s.T(), err)
}

func (s *ControllerTestSuite) TestProvisionsCertificatesForRoutes() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := testutil.NewInMemoryDataBroker(s.T())

	s.putRoute(ctx, client, "route-1", "https://app1.example.com")
	s.putRoute(ctx, client, "route-2", "https://app2.example.com")

	s.NoError(s.Client.Create(ctx, &pomerium_ingress_v1.Pomerium{
		ObjectMeta: meta_v1.ObjectMeta{Name: testGlobalSettingsName},
		Spec: pomerium_ingress_v1.PomeriumSpec{
			Secrets: testNamespace + "/secrets",
			CertificateAutoProvision: &pomerium_ingress_v1.CertificateAutoProvision{
				ClusterIssuer: new(testClusterIssuer),
			},
		},
	}))

	s.startController(ctx, client)

	s.eventuallyDNSNames(ctx, []string{"app1.example.com", "app2.example.com"})
}

func (s *ControllerTestSuite) TestRemovesCertificatesForDeletedRoutes() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := testutil.NewInMemoryDataBroker(s.T())

	s.putRoute(ctx, client, "route-1", "https://stays.example.com")
	s.putRoute(ctx, client, "route-2", "https://goes.example.com")

	s.NoError(s.Client.Create(ctx, &pomerium_ingress_v1.Pomerium{
		ObjectMeta: meta_v1.ObjectMeta{Name: testGlobalSettingsName},
		Spec: pomerium_ingress_v1.PomeriumSpec{
			Secrets: testNamespace + "/secrets",
			CertificateAutoProvision: &pomerium_ingress_v1.CertificateAutoProvision{
				ClusterIssuer: new(testClusterIssuer),
			},
		},
	}))

	s.startController(ctx, client)
	s.eventuallyDNSNames(ctx, []string{"goes.example.com", "stays.example.com"})

	_, err := client.Put(ctx, &databrokerpb.PutRequest{
		Records: []*databrokerpb.Record{{
			Type:      grpcutil.GetTypeURL(new(configpb.Route)),
			Id:        "route-2",
			DeletedAt: timestamppb.Now(),
		}},
	})
	require.NoError(s.T(), err)

	s.eventuallyDNSNames(ctx, []string{"stays.example.com"})
}

func (s *ControllerTestSuite) TestReplacesCertificatesWhenClusterIssuerChanges() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := testutil.NewInMemoryDataBroker(s.T())

	s.putRoute(ctx, client, "route-1", "https://app.example.com")

	pom := &pomerium_ingress_v1.Pomerium{
		ObjectMeta: meta_v1.ObjectMeta{Name: testGlobalSettingsName},
		Spec: pomerium_ingress_v1.PomeriumSpec{
			Secrets: testNamespace + "/secrets",
			CertificateAutoProvision: &pomerium_ingress_v1.CertificateAutoProvision{
				ClusterIssuer: new(testClusterIssuer),
			},
		},
	}
	s.NoError(s.Client.Create(ctx, pom))

	s.startController(ctx, client)
	s.eventuallyClusterIssuerNames(ctx, testClusterIssuer, []string{"app.example.com"})

	const newIssuer = "other-issuer"
	s.NoError(s.Client.Get(ctx, types.NamespacedName{Name: testGlobalSettingsName}, pom))
	pom.Spec.CertificateAutoProvision.ClusterIssuer = new(newIssuer)
	s.NoError(s.Client.Update(ctx, pom))

	s.eventuallyClusterIssuerNames(ctx, newIssuer, []string{"app.example.com"})
	s.eventuallyClusterIssuerNames(ctx, testClusterIssuer, nil)
}

func (s *ControllerTestSuite) TestRemovesCertificatesWhenClusterIssuerCleared() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := testutil.NewInMemoryDataBroker(s.T())

	s.putRoute(ctx, client, "route-1", "https://temp.example.com")

	pom := &pomerium_ingress_v1.Pomerium{
		ObjectMeta: meta_v1.ObjectMeta{Name: testGlobalSettingsName},
		Spec: pomerium_ingress_v1.PomeriumSpec{
			Secrets: testNamespace + "/secrets",
			CertificateAutoProvision: &pomerium_ingress_v1.CertificateAutoProvision{
				ClusterIssuer: new(testClusterIssuer),
			},
		},
	}
	s.NoError(s.Client.Create(ctx, pom))

	s.startController(ctx, client)
	s.eventuallyDNSNames(ctx, []string{"temp.example.com"})

	s.NoError(s.Client.Get(ctx, types.NamespacedName{Name: testGlobalSettingsName}, pom))
	pom.Spec.CertificateAutoProvision = nil
	s.NoError(s.Client.Update(ctx, pom))

	s.eventuallyDNSNames(ctx, nil)
}

func (s *ControllerTestSuite) eventuallyDNSNames(ctx context.Context, expected []string) {
	s.T().Helper()
	s.eventuallyClusterIssuerNames(ctx, testClusterIssuer, expected)
}

func (s *ControllerTestSuite) eventuallyClusterIssuerNames(ctx context.Context, clusterIssuer string, expected []string) {
	s.T().Helper()
	assert.Eventually(s.T(), func() bool {
		var list certmanager_v1.CertificateList
		if err := s.Client.List(ctx, &list, client.InNamespace(testNamespace)); err != nil {
			return false
		}
		got := make(map[string]bool)
		for _, c := range list.Items {
			if c.Spec.IssuerRef.Kind != "ClusterIssuer" || c.Spec.IssuerRef.Name != clusterIssuer {
				continue
			}
			for _, n := range c.Spec.DNSNames {
				got[n] = true
			}
		}
		if len(got) != len(expected) {
			return false
		}
		for _, n := range expected {
			if !got[n] {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond, "expected DNS names %v on Certificates with ClusterIssuer %q", expected, clusterIssuer)
}

func TestCertificateController(t *testing.T) {
	suite.Run(t, &ControllerTestSuite{})
}

// certManagerCRDPath returns the path to cert-manager's CRD yaml files in the
// local Go module cache.
func certManagerCRDPath(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/cert-manager/cert-manager").Output()
	require.NoError(t, err)
	return filepath.Join(strings.TrimSpace(string(out)), "deploy", "crds")
}

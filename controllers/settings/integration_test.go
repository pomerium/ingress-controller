package settings_test

import (
	context "context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/pomerium"
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
	s.NoError(icsv1.AddToScheme(scheme))

	useExistingCluster := false
	s.Environment = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		Scheme:                scheme,
		UseExistingCluster:    &useExistingCluster,
	}
	cfg, err := s.Environment.Start()
	s.NoError(err)
	require.NotNil(s.T(), cfg)

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	s.NoError(err)
	require.NotNil(s.T(), k8sClient)
	s.Client = k8sClient
}

func (s *ControllerTestSuite) SetupTest() {
	log.SetLogger(zapr.NewLogger(zaptest.NewLogger(s.T())))
}

func (s *ControllerTestSuite) deleteAll() {
	// s.Client.DeleteAll is not implemented for the test environment thus we need manually loop over objects
	ctx := context.Background()

	secrets := new(corev1.SecretList)
	s.NoError(s.Client.List(ctx, secrets))
	for i := range secrets.Items {
		s.NoError(s.Client.Delete(ctx, &secrets.Items[i]))
	}

	settings := new(icsv1.SettingsList)
	s.NoError(s.Client.List(ctx, settings))
	for i := range settings.Items {
		s.NoError(s.Client.Delete(ctx, &settings.Items[i]))
	}
}

func (s *ControllerTestSuite) TearDownTest() {
	s.deleteAll()
}

func (s *ControllerTestSuite) TearDownSuite() {
	s.NoError(s.Environment.Stop())
}

func (s *ControllerTestSuite) createTestController(ctx context.Context, reconciler pomerium.Reconciler, name types.NamespacedName) {
	mgr, err := ctrl.NewManager(s.Environment.Config, ctrl.Options{
		Scheme:             s.Environment.Scheme,
		MetricsBindAddress: "0",
	})
	s.NoError(err)
	s.NoError(settings.NewSettingsController(mgr, reconciler, name))

	go func() {
		if err = mgr.Start(ctx); err != nil && ctx.Err() == nil {
			s.T().Error(err)
		}
	}()
}

func (s *ControllerTestSuite) TestValidation() {
	auth := icsv1.Authenticate{
		URL: "https://provider.local",
	}
	idp := icsv1.IdentityProvider{
		Provider: "oidc",
		Secret:   "secret",
	}

	ctx := context.Background()
	for i, tc := range []struct {
		name        string
		spec        icsv1.SettingsSpec
		expectError bool
	}{
		{"empty spec", icsv1.SettingsSpec{}, true},
		{"ok spec", icsv1.SettingsSpec{
			Authenticate:     auth,
			IdentityProvider: idp,
		}, false},
		{"invalid auth url", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{URL: "hostname"},
			IdentityProvider: idp,
		}, true},
		{"auth required", icsv1.SettingsSpec{
			IdentityProvider: idp,
		}, true},
		{"idp required", icsv1.SettingsSpec{
			Authenticate: auth,
		}, true},
		{"idp secret required", icsv1.SettingsSpec{
			Authenticate: auth,
			IdentityProvider: icsv1.IdentityProvider{
				Secret:   "",
				Provider: "oidc",
			},
		}, true},
		{"idp provider required", icsv1.SettingsSpec{
			Authenticate: auth,
			IdentityProvider: icsv1.IdentityProvider{
				Secret:   "secret",
				Provider: "",
			},
		}, true},
		{"idp provider enum", icsv1.SettingsSpec{
			Authenticate: auth,
			IdentityProvider: icsv1.IdentityProvider{
				Secret:   "secret",
				Provider: "invalid",
			},
		}, true},
	} {
		s.T().Run(tc.name, func(t *testing.T) {
			err := s.Client.Create(ctx, &icsv1.Settings{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("settings-%d", i), Namespace: "default"},
				Spec:       tc.spec,
			})
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func (s *ControllerTestSuite) TestDependencies() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mc := controllers_mock.NewMockReconciler(gomock.NewController(s.T()))
	name := types.NamespacedName{Namespace: "pomerium", Name: "settings"}

	s.createTestController(ctx, mc, name)
}

func TestIngressController(t *testing.T) {
	suite.Run(t, &ControllerTestSuite{})
}

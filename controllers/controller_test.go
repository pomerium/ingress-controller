package controllers_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/ingress-controller/controllers"
	"github.com/pomerium/ingress-controller/model"
)

var (
	_ suite.SetupAllSuite     = &ControllerTestSuite{}
	_ suite.TearDownAllSuite  = &ControllerTestSuite{}
	_ suite.SetupTestSuite    = &ControllerTestSuite{}
	_ suite.TearDownTestSuite = &ControllerTestSuite{}
)

type mockPomeriumReconciler struct {
	upsert chan *model.IngressConfig
	delete chan types.NamespacedName
}

func newMockPomeriumReconciler() *mockPomeriumReconciler {
	return &mockPomeriumReconciler{
		upsert: make(chan *model.IngressConfig, 100),
		delete: make(chan types.NamespacedName, 100),
	}
}

func (m *mockPomeriumReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.upsert <- ic:
		return nil
	}
}

// Delete should delete pomerium routes corresponding to this ingress name
func (m *mockPomeriumReconciler) Delete(ctx context.Context, name types.NamespacedName) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.delete <- name:
		return nil
	}
}

type ControllerTestSuite struct {
	suite.Suite
	client.Client
	*envtest.Environment

	// created per test
	mgrCtxCancel context.CancelFunc
	mgrDone      chan error
	*mockPomeriumReconciler

	controllerName string
}

func (s *ControllerTestSuite) SetupSuite() {
	s.controllerName = "pomerium.io/ingress-controller"

	//logf.SetLogger(zapr.NewLogger(zap.NewDevelopment()))
	//zaptest.NewLogger(s.T()).Info("*** HELLO ZAP")

	s.Environment = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
	}

	cfg, err := s.Environment.Start()
	require.NoError(s.T(), err)
	require.NotNil(s.T(), cfg)

	scheme := runtime.NewScheme()
	require.NoError(s.T(), clientgoscheme.AddToScheme(scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(s.T(), err)
	require.NotNil(s.T(), k8sClient)
	s.Client = k8sClient
}

func (s *ControllerTestSuite) SetupTest() {
	logf.SetLogger(zapr.NewLogger(zaptest.NewLogger(s.T())))
	s.createTestController()
}

func (s *ControllerTestSuite) TearDownTest() {
	s.mgrCtxCancel()
	<-s.mgrDone

exhaust:
	for {
		select {
		case <-s.delete:
		case <-s.upsert:
		default:
			break exhaust
		}
	}
}

func (s *ControllerTestSuite) TearDownSuite() {
	require.NoError(s.T(), s.Environment.Stop())
}

func (s *ControllerTestSuite) createTestController() {
	mgr, err := ctrl.NewManager(s.Environment.Config, ctrl.Options{
		Scheme: s.Environment.Scheme,
	})
	require.NoError(s.T(), err)

	s.mockPomeriumReconciler = newMockPomeriumReconciler()
	err = (&controllers.Controller{
		PomeriumReconciler: s.mockPomeriumReconciler,
		Client:             s.Client,
		Registry:           model.NewRegistry(),
		EventRecorder:      mgr.GetEventRecorderFor("Ingress"),
	}).SetupWithManager(mgr)
	require.NoError(s.T(), err)

	ctx, cancel := context.WithCancel(context.Background())
	s.mgrCtxCancel = cancel
	s.mgrDone = make(chan error)

	go func() {
		s.mgrDone <- mgr.Start(ctx)
	}()
}

func (s *ControllerTestSuite) makeIngressClass(isDefault bool) *networkingv1.IngressClass {
	s.T().Helper()
	ctx := context.Background()
	ic := &networkingv1.IngressClass{
		ObjectMeta: v1.ObjectMeta{
			Name:        "pomerium",
			Namespace:   "default",
			Annotations: map[string]string{},
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: s.controllerName,
		},
	}
	if isDefault {
		ic.Annotations[controllers.IngressClassDefault] = "true"
	}

	require.NoError(s.T(), s.Client.Create(ctx, ic))
	return ic
}

func (s *ControllerTestSuite) setIngressClassDefault(ic *networkingv1.IngressClass, isDefault bool) {
	s.T().Helper()
	patch := client.MergeFrom(&networkingv1.IngressClass{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				controllers.IngressClassDefault: fmt.Sprintf("%v", isDefault),
			},
		},
	})
	require.NoError(s.T(), s.Client.Patch(context.Background(), ic, patch))
}

func (s *ControllerTestSuite) setIngressClass(ing *networkingv1.Ingress, ingressClass string) {
	s.T().Helper()
	patch := client.MergeFrom(&networkingv1.Ingress{
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClass,
		},
	})
	require.NoError(s.T(), s.Client.Patch(context.Background(), ing, patch))
}

func (s *ControllerTestSuite) makeIngress(count int) ([]*networkingv1.Ingress, func()) {
	s.T().Helper()
	ctx := context.Background()
	typePrefix := networkingv1.PathTypePrefix
	var ingress []*networkingv1.Ingress
	for i := 0; i < count; i++ {
		ingress = append(ingress, &networkingv1.Ingress{
			TypeMeta:   v1.TypeMeta{},
			ObjectMeta: v1.ObjectMeta{Name: fmt.Sprintf("ingress-%d", i), Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{
					Host: fmt.Sprintf("host-%d.localhost.pomerium.io", i),
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/",
								PathType: &typePrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "hello",
										Port: networkingv1.ServiceBackendPort{
											Number: 80,
										},
									},
								},
							}},
						},
					},
				}},
			},
		})
	}

	cleanup := func() {
		for _, ing := range ingress {
			assert.NoError(s.T(), s.Client.Delete(ctx, ing))
		}
	}

	for _, ing := range ingress {
		if err := s.Client.Create(ctx, ing); err != nil {
			cleanup()
			require.NoError(s.T(), err)
		}
	}

	return ingress, cleanup
}

func (s *ControllerTestSuite) getUpsert() *model.IngressConfig {
	s.T().Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	select {
	case <-ctx.Done():
		s.T().Fatal("timed out waiting for Upsert")
		return nil
	case ic := <-s.mockPomeriumReconciler.upsert:
		return ic
	}
}

func (s *ControllerTestSuite) getDelete() types.NamespacedName {
	s.T().Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	select {
	case <-ctx.Done():
		s.T().Fatal("timed out waiting for Delete")
		return types.NamespacedName{}
	case name := <-s.mockPomeriumReconciler.delete:
		return name
	}
}

func (s *ControllerTestSuite) Eventually(diffFn func() string) {
	s.T().Helper()
	var diff string
	if !assert.Eventually(s.T(), func() bool {
		diff = diffFn()
		return diff == ""
	}, time.Second*30, time.Millisecond*200) {
		require.Empty(s.T(), diff)
	}
}

func (s ControllerTestSuite) Equal(a, b interface{}) {
	s.T().Helper()
	if diff := cmp.Diff(a, b); diff != "" {
		s.T().Fatal(diff)
	}
}

func (s *ControllerTestSuite) TestIngressClass() {
	/*
		TEST PLAN

		- create ingresses
		-
		- no ingresses should be picked up for reconciliation
		- change the controller to be default ingress controller
		- ingresses should get created
		- update the controller to no longer be default ingress controller
		- ingresses should be deleted
		- set ingress controller class name for some of the ingress resources
		- only those ingresses should be created
	*/

	ctx := context.Background()
	ingress, cleanup := s.makeIngress(2)
	defer cleanup()

	// no ingresses should be picked up for reconciliation as there's no ingress class record
	s.assertNoReconciliations(0)

	// create ingress controller spec that is not default
	ingressClass := s.makeIngressClass(false)

	// no ingresses should be picked up for reconciliation as we are not default
	s.assertNoReconciliations(0)

	// assign ingressClass to one of the ingresses
	// it should be picked up
	ingress[0].Spec.IngressClassName = &ingressClass.Name
	require.NoError(s.T(), s.Client.Update(ctx, ingress[0]))
	ic := s.getUpsert()
	s.Equal(ic.Ingress.Name, ingress[0].Name)
	s.assertNoReconciliations(0)

	s.setIngressClassDefault(ingressClass, true)
	var ingressNames []string
	for i := 0; i < len(ingress); i++ {
		ic := s.getUpsert()
		ingressNames = append(ingressNames, ic.Ingress.Name)
	}
	require.ElementsMatch(s.T(), nil, ingressNames)
}

// TestDependencies verifies that when objects the Ingress depends on change,
// a configuration reconciliation would happen
func (s *ControllerTestSuite) TestDependencies() {
	id := uuid.NewString()
	svcName := types.NamespacedName{Name: fmt.Sprintf("svc-%s", id), Namespace: "default"}
	secretName := types.NamespacedName{Name: fmt.Sprintf("secret-%s", id), Namespace: "default"}
	ingressName := types.NamespacedName{Name: fmt.Sprintf("ingress-%s", id), Namespace: "default"}

	service := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      svcName.Name,
			Namespace: svcName.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.IntOrString{IntVal: 80},
			}},
		},
		Status: corev1.ServiceStatus{},
	}

	typePrefix := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: v1.ObjectMeta{Name: ingressName.Name, Namespace: ingressName.Namespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "service.localhost.pomerium.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &typePrefix,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: svcName.Name,
									Port: networkingv1.ServiceBackendPort{
										Name: "http",
									},
								},
							},
						}},
					},
				},
			}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	require.NoError(s.T(), s.Client.Create(ctx, ingress))
	s.assertNoReconciliations(time.Second)

	require.NoError(s.T(), s.Client.Create(ctx, service))

	ic := s.getUpsert()
	s.Equal(service, ic.Services[svcName])
	s.assertNoReconciliations(time.Second)

	service.Spec.Ports[0].Port = 8080
	require.NoError(s.T(), s.Client.Update(ctx, service))

	ic = s.getUpsert()
	s.Equal(service, ic.Services[svcName])
	s.assertNoReconciliations(time.Second)

	ingress = ic.Ingress.DeepCopy()
	ingress.Spec.TLS = append(ingress.Spec.TLS, networkingv1.IngressTLS{
		Hosts:      []string{"service.localhost.pomerium.io"},
		SecretName: secretName.Name,
	})
	require.NoError(s.T(), s.Client.Update(ctx, ingress))
	s.assertNoReconciliations(time.Second)

	secret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      secretName.Name,
			Namespace: "default",
		},
		Data: map[string][]byte{
			corev1.TLSPrivateKeyKey: []byte("A"),
			corev1.TLSCertKey:       []byte("A"),
		},
		Type: corev1.SecretTypeTLS,
	}

	require.NoError(s.T(), s.Client.Create(ctx, secret))
	ic = s.getUpsert()
	s.Equal(secret, ic.Secrets[secretName])
	s.assertNoReconciliations(time.Second)

	secret.Data = map[string][]byte{
		corev1.TLSPrivateKeyKey: []byte("B"),
		corev1.TLSCertKey:       []byte("B"),
	}
	require.NoError(s.T(), s.Client.Update(ctx, secret))
	ic = s.getUpsert()
	s.Equal(secret, ic.Secrets[secretName])
	s.assertNoReconciliations(time.Second)
}

func (s *ControllerTestSuite) assertNoReconciliations(waitFor time.Duration) {
	select {
	case ic := <-s.mockPomeriumReconciler.upsert:
		s.T().Fatal("unexpected upsert", ic)
	case name := <-s.mockPomeriumReconciler.delete:
		s.T().Fatal("unexpected delete", name)
	case <-time.After(waitFor + time.Millisecond*100):
		return
	default:
		return
	}
}

func TestIngressController(t *testing.T) {
	suite.Run(t, &ControllerTestSuite{})
}

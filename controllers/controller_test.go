package controllers_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	allNamespaces []string = nil

	cmpOpts = cmpopts.IgnoreTypes(metav1.TypeMeta{})
)

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

type mockPomeriumReconciler struct {
	sync.RWMutex
	lastUpsert *model.IngressConfig
	lastDelete *types.NamespacedName
}

func (m *mockPomeriumReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	m.Lock()
	defer m.Unlock()

	m.lastUpsert = ic
	m.lastDelete = nil
	return true, nil
}

func (m *mockPomeriumReconciler) Delete(ctx context.Context, name types.NamespacedName) error {
	m.Lock()
	defer m.Unlock()

	m.lastDelete = &name
	m.lastUpsert = nil
	return nil
}

func (m *mockPomeriumReconciler) DeleteAll(ctx context.Context) error {
	panic("not implemented")
}

func (s *ControllerTestSuite) EventuallyDeleted(name types.NamespacedName) {
	s.T().Helper()
	require.Eventually(s.T(), func() bool {
		s.mockPomeriumReconciler.Lock()
		defer s.mockPomeriumReconciler.Unlock()

		if s.mockPomeriumReconciler.lastDelete == nil {
			return false
		}
		val := *s.mockPomeriumReconciler.lastDelete == name
		s.mockPomeriumReconciler.lastDelete = nil
		return val
	}, time.Second, time.Millisecond*50, "lastDeleted != %s", name)
}

func (s *ControllerTestSuite) diffFn(diffFn func(current *model.IngressConfig) string, diff *string) func() bool {
	return func() bool {
		s.mockPomeriumReconciler.RLock()
		defer s.mockPomeriumReconciler.RUnlock()

		if s.lastUpsert == nil {
			*diff = "lastUpsert == nil"
			return false
		}
		if s.lastDelete != nil {
			*diff = fmt.Sprintf("lastDelete = %s", *s.lastDelete)
		}
		*diff = diffFn(s.lastUpsert)
		return *diff == ""
	}
}

func (s *ControllerTestSuite) EventuallyUpsert(diffFn func(current *model.IngressConfig) string, msg string) {
	s.T().Helper()
	var diff string

	if !assert.Eventually(s.T(), s.diffFn(diffFn, &diff), time.Second*30, time.Millisecond*50) {
		s.T().Fatalf("condition %q never satisfied: %s", msg, diff)
	}
}

func (s *ControllerTestSuite) NeverEqual(diffFn func(current *model.IngressConfig) string) {
	s.T().Helper()
	var diff string
	if !assert.Never(s.T(), s.diffFn(diffFn, &diff), time.Second, time.Millisecond*50) {
		s.T().Fatal("became equal")
	}
}

func (s *ControllerTestSuite) NoError(err error) {
	s.T().Helper()
	require.NoError(s.T(), err)
}

func (s *ControllerTestSuite) SetupSuite() {
	s.controllerName = "pomerium.io/ingress-controller"

	scheme := runtime.NewScheme()
	s.NoError(clientgoscheme.AddToScheme(scheme))

	useExistingCluster := false
	s.Environment = &envtest.Environment{
		Scheme:             scheme,
		UseExistingCluster: &useExistingCluster,
	}
	cfg, err := s.Environment.Start()
	s.NoError(err)
	require.NotNil(s.T(), cfg)
	s.T().Logf("API Host: %s", cfg.Host)

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	s.NoError(err)
	require.NotNil(s.T(), k8sClient)
	s.Client = k8sClient
}

func (s *ControllerTestSuite) SetupTest() {
	logf.SetLogger(zapr.NewLogger(zaptest.NewLogger(s.T())))
}

func (s *ControllerTestSuite) deleteAll() {
	// s.Client.DeleteAll is not implemented for the test environment thus we need manually loop over objects
	ctx := context.Background()

	icl := new(networkingv1.IngressClassList)
	s.NoError(s.Client.List(ctx, icl))
	for i := range icl.Items {
		s.NoError(s.Client.Delete(ctx, &icl.Items[i]))
	}

	il := new(networkingv1.IngressList)
	s.NoError(s.Client.List(ctx, il))
	for i := range il.Items {
		s.NoError(s.Client.Delete(ctx, &il.Items[i]))
	}

	svcs := new(corev1.ServiceList)
	s.NoError(s.Client.List(ctx, svcs))
	for i := range svcs.Items {
		s.NoError(s.Client.Delete(ctx, &svcs.Items[i]))
	}

	secrets := new(corev1.SecretList)
	s.NoError(s.Client.List(ctx, secrets))
	for i := range secrets.Items {
		s.NoError(s.Client.Delete(ctx, &secrets.Items[i]))
	}
}

func (s *ControllerTestSuite) TearDownTest() {
	s.mgrCtxCancel()
	<-s.mgrDone
	s.deleteAll()
}

func (s *ControllerTestSuite) TearDownSuite() {
	s.NoError(s.Environment.Stop())
}

func (s *ControllerTestSuite) createTestController(ctx context.Context, namespaces []string) {
	s.mockPomeriumReconciler = &mockPomeriumReconciler{}
	mgr, err := controllers.NewIngressController(s.Environment.Config,
		ctrl.Options{
			Scheme: s.Environment.Scheme,
		},
		s.mockPomeriumReconciler,
		namespaces)
	s.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	s.mgrCtxCancel = cancel
	s.mgrDone = make(chan error)

	go func() {
		s.mgrDone <- mgr.Start(ctx)
	}()
}

func (s *ControllerTestSuite) initialTestObjects(namespace string) (
	*networkingv1.IngressClass,
	*networkingv1.Ingress,
	*corev1.Service,
	*corev1.Secret,
) {
	typePrefix := networkingv1.PathTypePrefix
	icsName := "pomerium"
	return &networkingv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: icsName, Namespace: namespace},
			Spec: networkingv1.IngressClassSpec{
				Controller: s.controllerName,
			},
		},
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: namespace},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &icsName,
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"service.localhost.pomerium.io"},
					SecretName: "secret",
				}},
				Rules: []networkingv1.IngressRule{{
					Host: "service.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/",
								PathType: &typePrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "service",
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
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service",
				Namespace: namespace,
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
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				corev1.TLSPrivateKeyKey: []byte("A"),
				corev1.TLSCertKey:       []byte("A"),
			},
			Type: corev1.SecretTypeTLS,
		}
}

func (s *ControllerTestSuite) TestIngressClass() {
	ctx := context.Background()
	s.createTestController(ctx, allNamespaces)

	ingressClass, ingress, service, _ := s.initialTestObjects("default")
	ingress.Spec.TLS = nil
	ingress.Spec.IngressClassName = nil
	// ingress should not be picked up for reconciliation as there's no ingress class record
	s.NoError(s.Client.Create(ctx, ingress))
	s.NoError(s.Client.Create(ctx, service))
	s.NeverEqual(func(ic *model.IngressConfig) string {
		return cmp.Diff(ingress, ic.Ingress, cmpOpts)
	})

	// create ingress controller spec that is not default
	s.NoError(s.Client.Create(ctx, ingressClass))
	s.NeverEqual(func(ic *model.IngressConfig) string {
		return cmp.Diff(ingress, ic.Ingress, cmpOpts)
	})

	// mark ingress with ingress class name
	ingress.Spec.IngressClassName = &ingressClass.Name
	s.NoError(s.Client.Update(ctx, ingress))
	s.EventuallyUpsert(func(ic *model.IngressConfig) string {
		return cmp.Diff(ingress, ic.Ingress, cmpOpts)
	}, "set ingressClass to ingress spec")

	// remove ingress class annotation, it should be deleted
	ingress.Spec.IngressClassName = nil
	s.NoError(s.Client.Update(ctx, ingress))
	s.EventuallyDeleted(types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace})

	// make ingressClass default, ingress should be recreated
	ingressClass.Annotations = map[string]string{controllers.IngressClassDefaultAnnotationKey: "true"}
	s.NoError(s.Client.Update(ctx, ingressClass))
	s.EventuallyUpsert(func(ic *model.IngressConfig) string {
		return cmp.Diff(ingress, ic.Ingress, cmpOpts)
	}, "default ingress class")
}

// TestDependencies verifies that when objects the Ingress depends on change,
// a configuration reconciliation would happen
func (s *ControllerTestSuite) TestDependencies() {
	ctx := context.Background()
	s.createTestController(ctx, allNamespaces)

	ingressClass, ingress, service, secret := s.initialTestObjects("default")
	svcName := types.NamespacedName{Name: "service", Namespace: "default"}
	secretName := types.NamespacedName{Name: "secret", Namespace: "default"}

	for _, obj := range []client.Object{ingress, service, secret} {
		s.NoError(s.Client.Create(ctx, obj))
		s.NeverEqual(func(ic *model.IngressConfig) string {
			return cmp.Diff(ingress, ic.Ingress)
		})
	}
	s.NoError(s.Client.Create(ctx, ingressClass))
	s.EventuallyUpsert(func(ic *model.IngressConfig) string {
		return cmp.Diff(service, ic.Services[svcName], cmpOpts) +
			cmp.Diff(secret, ic.Secrets[secretName], cmpOpts) +
			cmp.Diff(ingress, ic.Ingress, cmpOpts)
	}, "secret, service, ingress up to date")

	service.Spec.Ports[0].Port = 8080
	s.NoError(s.Client.Update(ctx, service))
	s.EventuallyUpsert(func(ic *model.IngressConfig) string {
		return cmp.Diff(service, ic.Services[svcName], cmpOpts)
	}, "updated port")

	// update secret
	secret.Data = map[string][]byte{
		corev1.TLSPrivateKeyKey: []byte("B"),
		corev1.TLSCertKey:       []byte("B"),
	}
	s.NoError(s.Client.Update(ctx, secret))
	s.EventuallyUpsert(func(ic *model.IngressConfig) string {
		return cmp.Diff(secret, ic.Secrets[secretName], cmpOpts)
	}, "updated secret")
}

// TestNamespaces checks that controller would only
func (s *ControllerTestSuite) TestNamespaces() {
	namespaces := map[string]bool{"a": true, "b": false, "c": true, "d": false}

	ctx := context.Background()
	s.createTestController(ctx, []string{"a", "c"})
	del := func(obj client.Object) { s.Client.Delete(ctx, obj) }

	ingressClass, _, _, _ := s.initialTestObjects("")
	s.NoError(s.Client.Create(ctx, ingressClass))

	for ns, shouldCreate := range namespaces {
		_, ingress, service, secret := s.initialTestObjects(ns)
		for _, obj := range []client.Object{
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}},
			ingress, service, secret,
		} {
			s.T().Logf("%s/%s %s", obj.GetNamespace(), obj.GetName(), reflect.TypeOf(obj))
			s.NoError(s.Client.Create(ctx, obj))
			defer del(obj)
		}

		diffFn := func(ic *model.IngressConfig) string {
			return cmp.Diff(ingress, ic.Ingress, cmpOpts)
		}

		if shouldCreate {
			s.EventuallyUpsert(diffFn, ns)
		} else {
			s.NeverEqual(diffFn)
		}
	}
}

func TestIngressController(t *testing.T) {
	suite.Run(t, &ControllerTestSuite{})
}

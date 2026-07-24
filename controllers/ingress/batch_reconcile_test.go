package ingress

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/pomerium/ingress-controller/controllers/reporter"
	"github.com/pomerium/ingress-controller/model"
)

type batchTestReconciler struct {
	sync.Mutex
	tracked  map[types.NamespacedName]int64
	setCalls int
	setErr   error
	lastSet  []types.NamespacedName
	reported map[types.NamespacedName]int
}

func (*batchTestReconciler) Upsert(context.Context, *model.IngressConfig) (bool, error) {
	return false, errors.New("unexpected Upsert")
}

func (*batchTestReconciler) Delete(context.Context, types.NamespacedName) (bool, error) {
	return false, errors.New("unexpected Delete")
}

func (r *batchTestReconciler) Set(_ context.Context, ics []*model.IngressConfig) (bool, error) {
	r.Lock()
	defer r.Unlock()
	r.setCalls++
	if r.setErr != nil {
		return false, r.setErr
	}
	r.tracked = make(map[types.NamespacedName]int64, len(ics))
	r.lastSet = r.lastSet[:0]
	for _, ic := range ics {
		name := ic.GetIngressNamespacedName()
		r.tracked[name] = ic.Ingress.Generation
		r.lastSet = append(r.lastSet, name)
	}
	sort.Slice(r.lastSet, func(i, j int) bool { return r.lastSet[i].String() < r.lastSet[j].String() })
	return true, nil
}

func (r *batchTestReconciler) NeedsIngressUpdate(ic *model.IngressConfig) (bool, error) {
	r.Lock()
	defer r.Unlock()
	generation, ok := r.tracked[ic.GetIngressNamespacedName()]
	return !ok || generation != ic.Ingress.Generation, nil
}

func (r *batchTestReconciler) TracksIngress(name types.NamespacedName) bool {
	r.Lock()
	defer r.Unlock()
	_, ok := r.tracked[name]
	return ok
}

func (r *batchTestReconciler) IngressReconciled(_ context.Context, ingress *networkingv1.Ingress) error {
	r.Lock()
	defer r.Unlock()
	if r.reported == nil {
		r.reported = make(map[types.NamespacedName]int)
	}
	r.reported[types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}]++
	return nil
}

func (*batchTestReconciler) IngressNotReconciled(context.Context, *networkingv1.Ingress, error) error {
	return nil
}

func (*batchTestReconciler) IngressDeleted(context.Context, types.NamespacedName, string) error {
	return nil
}

func newBatchTestController(
	t *testing.T,
	reconciler *batchTestReconciler,
) (*ingressController, *networkingv1.Ingress) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))

	className := "pomerium"
	pathType := networkingv1.PathTypePrefix
	newIngress := func(name string) *networkingv1.Ingress {
		return &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Generation: 2},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &className,
				Rules: []networkingv1.IngressRule{{
					Host: name + ".example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
								Name: "echo",
								Port: networkingv1.ServiceBackendPort{Name: "http"},
							}},
						}},
					}},
				}},
			},
		}
	}

	a := newIngress("app-a")
	b := newIngress("app-b")
	objects := []runtime.Object{
		&networkingv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: className},
			Spec:       networkingv1.IngressClassSpec{Controller: DefaultClassControllerName},
		},
		a,
		b,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
				Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080),
			}}},
		},
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	r := &ingressController{
		controllerName:             DefaultClassControllerName,
		annotationPrefix:           DefaultAnnotationPrefix,
		Scheme:                     scheme,
		Client:                     c,
		IngressReconciler:          reconciler,
		Registry:                   model.NewRegistry(),
		MultiIngressStatusReporter: reporter.MultiIngressStatusReporter{reconciler},
		ingressChangeDetector:      reconciler,
		ingressKind:                "Ingress",
		serviceKind:                "Service",
		endpointsKind:              "Endpoints",
		secretKind:                 "Secret",
		ingressClassKind:           "IngressClass",
	}
	r.batchCoordinator = newReconcileBatchCoordinator(time.Millisecond, 10*time.Millisecond, func(ctx context.Context) error {
		_, err := r.reconcileAll(ctx)
		return err
	})
	startBatchCoordinator(t, r.batchCoordinator)
	return r, b
}

func TestReconcileBatchedAppliesFullStateOnce(t *testing.T) {
	reconciler := &batchTestReconciler{tracked: map[types.NamespacedName]int64{
		{Namespace: "default", Name: "app-a"}: 1,
		{Namespace: "default", Name: "app-b"}: 1,
	}}
	r, _ := newBatchTestController(t, reconciler)
	ctx := context.Background()

	result, err := r.reconcileBatched(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
		Namespace: "default", Name: "app-a",
	}})
	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, 1, reconciler.setCalls)
	assert.Equal(t, []types.NamespacedName{
		{Namespace: "default", Name: "app-a"},
		{Namespace: "default", Name: "app-b"},
	}, reconciler.lastSet)
	assert.Equal(t, 1, reconciler.reported[types.NamespacedName{Namespace: "default", Name: "app-a"}])

	result, err = r.reconcileBatched(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
		Namespace: "default", Name: "app-b",
	}})
	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, 1, reconciler.setCalls, "the first full-state Set already included app-b")
	assert.Zero(t, reconciler.reported[types.NamespacedName{Namespace: "default", Name: "app-b"}],
		"an unchanged ingress should not emit duplicate status updates")
}

func TestReconcileBatchedSkipsUnrelatedIngressWithMissingDependency(t *testing.T) {
	reconciler := &batchTestReconciler{tracked: map[types.NamespacedName]int64{
		{Namespace: "default", Name: "app-a"}: 1,
		{Namespace: "default", Name: "app-b"}: 1,
	}}
	r, ingressB := newBatchTestController(t, reconciler)
	broken := ingressB.DeepCopy()
	broken.Name = "broken"
	broken.ResourceVersion = ""
	broken.UID = ""
	broken.Spec.Rules[0].Host = "broken.example.com"
	broken.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name = "missing"
	require.NoError(t, r.Client.Create(context.Background(), broken))

	result, err := r.reconcileBatched(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Namespace: "default", Name: "app-a",
	}})
	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, 1, reconciler.setCalls)
	assert.Equal(t, []types.NamespacedName{
		{Namespace: "default", Name: "app-a"},
		{Namespace: "default", Name: "app-b"},
	}, reconciler.lastSet)

	result, err = r.reconcileBatched(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Namespace: "default", Name: "broken",
	}})
	require.ErrorContains(t, err, "missing")
	assert.True(t, result.Requeue)
	assert.Equal(t, 1, reconciler.setCalls, "the invalid ingress must not trigger another full Set")
}

func TestReconcileBatchedDeleteRetriesFailedSet(t *testing.T) {
	reconciler := &batchTestReconciler{tracked: map[types.NamespacedName]int64{
		{Namespace: "default", Name: "app-a"}: 2,
		{Namespace: "default", Name: "app-b"}: 2,
	}}
	r, ingressB := newBatchTestController(t, reconciler)
	ctx := context.Background()
	require.NoError(t, r.Client.Delete(ctx, ingressB))

	reconciler.setErr = errors.New("temporary failure")
	result, err := r.reconcileBatched(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
		Namespace: "default", Name: "app-b",
	}})
	require.ErrorContains(t, err, "temporary failure")
	assert.True(t, result.Requeue)
	assert.Equal(t, 1, reconciler.setCalls)
	assert.True(t, reconciler.TracksIngress(types.NamespacedName{Namespace: "default", Name: "app-b"}))

	reconciler.setErr = nil
	result, err = r.reconcileBatched(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
		Namespace: "default", Name: "app-b",
	}})
	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, 2, reconciler.setCalls)
	assert.False(t, reconciler.TracksIngress(types.NamespacedName{Namespace: "default", Name: "app-b"}))
	assert.Equal(t, []types.NamespacedName{{Namespace: "default", Name: "app-a"}}, reconciler.lastSet)
}

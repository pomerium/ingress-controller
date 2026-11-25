package ingress

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFetchIngressServiceEndpointSlices(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	slice := func(name, namespace, service string) *discoveryv1.EndpointSlice {
		return &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    map[string]string{discoveryv1.LabelServiceName: service},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "no-slices", Namespace: "ns"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "ns"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName, ExternalName: "example.com"},
		},
		slice("svc-aaaaa", "ns", "svc"),
		slice("svc-bbbbb", "ns", "svc"),
		slice("other-ccccc", "ns", "other"),
		slice("svc-ddddd", "another-ns", "svc"),
	).Build()

	services := map[types.NamespacedName]*corev1.Service{}
	slices := map[types.NamespacedName][]*discoveryv1.EndpointSlice{}
	for _, name := range []string{"svc", "no-slices", "external"} {
		require.NoError(t, fetchIngressService(context.Background(), c, services, slices, types.NamespacedName{Namespace: "ns", Name: name}))
	}

	var got []string
	for _, s := range slices[types.NamespacedName{Namespace: "ns", Name: "svc"}] {
		got = append(got, s.Name)
	}
	assert.ElementsMatch(t, []string{"svc-aaaaa", "svc-bbbbb"}, got, "only the slices of the service, in its namespace")
	assert.Empty(t, slices[types.NamespacedName{Namespace: "ns", Name: "no-slices"}], "a service without slices is not an error")
	assert.NotContains(t, slices, types.NamespacedName{Namespace: "ns", Name: "external"}, "ExternalName services have no endpoints")
	assert.Len(t, services, 3)
}

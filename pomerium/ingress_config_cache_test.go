package pomerium

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
)

func cacheTestIngressConfig() *model.IngressConfig {
	serviceName := types.NamespacedName{Namespace: "default", Name: "service"}
	secretName := types.NamespacedName{Namespace: "default", Name: "tls"}
	return &model.IngressConfig{
		AnnotationPrefix: "ingress.pomerium.io",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default", Name: "app", UID: "ingress-uid", Generation: 7,
				ResourceVersion: "100", Labels: map[string]string{"app": "test"},
				Annotations: map[string]string{"ingress.pomerium.io/pass_identity_headers": "true"},
			},
			Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "app.example.test"}}},
			Status: networkingv1.IngressStatus{LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{{IP: "192.0.2.1"}},
			}},
		},
		Services: map[types.NamespacedName]*corev1.Service{serviceName: {
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "service", UID: "service-uid", ResourceVersion: "200"},
			Spec:       corev1.ServiceSpec{ClusterIP: "10.96.0.10"},
		}},
		Endpoints: map[types.NamespacedName]*corev1.Endpoints{serviceName: {
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "service", UID: "endpoints-uid", ResourceVersion: "300"},
			Subsets:    []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}},
		}},
		Secrets: map[types.NamespacedName]*corev1.Secret{secretName: {
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "tls", UID: "secret-uid", ResourceVersion: "400"},
			Type:       corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: []byte("certificate")},
		}},
	}
}

func TestIngressConfigCacheIgnoresStatusOnlyUpdates(t *testing.T) {
	before := cacheTestIngressConfig()
	after := cacheTestIngressConfig()
	after.Ingress.ResourceVersion = "101"
	after.Ingress.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "controller"}}
	after.Ingress.Status.LoadBalancer.Ingress[0].IP = "192.0.2.2"

	beforeFingerprint, err := fingerprintIngressConfig(before)
	require.NoError(t, err)
	afterFingerprint, err := fingerprintIngressConfig(after)
	require.NoError(t, err)
	assert.Equal(t, beforeFingerprint, afterFingerprint)
}

func TestIngressConfigCacheDetectsConfigInputs(t *testing.T) {
	base, err := fingerprintIngressConfig(cacheTestIngressConfig())
	require.NoError(t, err)
	tests := map[string]func(*model.IngressConfig){
		"annotation": func(ic *model.IngressConfig) {
			ic.Ingress.Annotations["ingress.pomerium.io/pass_identity_headers"] = "false"
		},
		"label": func(ic *model.IngressConfig) { ic.Ingress.Labels["app"] = "changed" },
		"spec":  func(ic *model.IngressConfig) { ic.Ingress.Spec.Rules[0].Host = "changed.example.test" },
		"service": func(ic *model.IngressConfig) {
			ic.Services[types.NamespacedName{Namespace: "default", Name: "service"}].Spec.ClusterIP = "10.96.0.11"
		},
		"endpoints": func(ic *model.IngressConfig) {
			ic.Endpoints[types.NamespacedName{Namespace: "default", Name: "service"}].Subsets[0].Addresses[0].IP = "10.0.0.2"
		},
		"secret": func(ic *model.IngressConfig) {
			ic.Secrets[types.NamespacedName{Namespace: "default", Name: "tls"}].Data[corev1.TLSCertKey] = []byte("changed")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			ic := cacheTestIngressConfig()
			mutate(ic)
			got, err := fingerprintIngressConfig(ic)
			require.NoError(t, err)
			assert.NotEqual(t, base, got)
		})
	}
}

func TestIngressConfigCacheShortCircuitsDuplicates(t *testing.T) {
	ic := cacheTestIngressConfig()
	fingerprint, err := fingerprintIngressConfig(ic)
	require.NoError(t, err)
	r := &DataBrokerReconciler{}
	r.ingressConfigCache.store(ic.GetIngressNamespacedName(), fingerprint)

	changed, err := r.Upsert(context.Background(), cacheTestIngressConfig())
	require.NoError(t, err)
	assert.False(t, changed)

	r.ingressConfigCache.replace(map[types.NamespacedName]ingressConfigFingerprint{ic.GetIngressNamespacedName(): fingerprint})
	changed, err = r.Set(context.Background(), []*model.IngressConfig{cacheTestIngressConfig()})
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestIngressConfigCacheDoesNotSkipInitialEmptySet(t *testing.T) {
	var cache ingressConfigCache
	assert.False(t, cache.matches(map[types.NamespacedName]ingressConfigFingerprint{}))
	cache.replace(map[types.NamespacedName]ingressConfigFingerprint{})
	assert.True(t, cache.matches(map[types.NamespacedName]ingressConfigFingerprint{}))
}

func TestIngressConfigCacheTracksAndDeletes(t *testing.T) {
	ic := cacheTestIngressConfig()
	fingerprint, err := fingerprintIngressConfig(ic)
	require.NoError(t, err)
	var cache ingressConfigCache
	name := ic.GetIngressNamespacedName()
	cache.store(name, fingerprint)
	assert.True(t, cache.contains(name))
	cache.delete(name)
	assert.False(t, cache.contains(name))
}

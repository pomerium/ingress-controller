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
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/pomerium/ingress-controller/model"

	_ "github.com/pomerium/ingress-controller/internal"
)

// validIngressConfig returns a minimal, valid IngressConfig for the given name/host.
func validIngressConfig(name, host string) *model.IngressConfig {
	pathTypePrefix := networkingv1.PathTypePrefix
	return &model.IngressConfig{
		AnnotationPrefix: "ingress.pomerium.io",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/",
								PathType: &pathTypePrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "service",
										Port: networkingv1.ServiceBackendPort{Name: "http"},
									},
								},
							}},
						},
					},
				}},
			},
		},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{Name: "service", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Protocol:   corev1.ProtocolTCP,
						Port:       80,
						TargetPort: intstr.FromInt(80),
					}},
				},
			},
		},
	}
}

// invalidIngressConfig returns an IngressConfig that fails route generation
// (empty host without the allow-empty-host annotation).
func invalidIngressConfig(name string) *model.IngressConfig {
	ic := validIngressConfig(name, "")
	return ic
}

func TestBuildConfigCheap_AllValid(t *testing.T) {
	ctx := context.Background()
	ics := []*model.IngressConfig{
		validIngressConfig("a", "a.localhost.pomerium.io"),
		validIngressConfig("b", "b.localhost.pomerium.io"),
	}

	cfg := buildConfigCheap(ctx, ics)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.GetRoutes(), 2, "both valid ingresses should produce a route")
}

func TestBuildConfigCheap_SkipsInvalid(t *testing.T) {
	ctx := context.Background()
	ics := []*model.IngressConfig{
		validIngressConfig("a", "a.localhost.pomerium.io"),
		invalidIngressConfig("bad"), // empty host -> route generation fails
		validIngressConfig("c", "c.localhost.pomerium.io"),
	}

	cfg := buildConfigCheap(ctx, ics)
	require.NotNil(t, cfg)
	// The invalid ingress is skipped; the two valid ones remain.
	assert.Len(t, cfg.GetRoutes(), 2, "invalid ingress should be skipped, valid ones kept")

	hosts := map[string]bool{}
	for _, r := range cfg.GetRoutes() {
		hosts[r.GetFrom()] = true
	}
	assert.True(t, hosts["https://a.localhost.pomerium.io"], "route a should be present")
	assert.True(t, hosts["https://c.localhost.pomerium.io"], "route c should be present")
}

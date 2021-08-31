package pomerium

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/pomerium/ingress-controller/model"
)

func TestHttp01Solver(t *testing.T) {
	ptype := networkingv1.PathTypeExact
	routes, err := ingressToRoutes(context.Background(), &model.IngressConfig{
		Ingress: &networkingv1.Ingress{
			ObjectMeta: v1meta.ObjectMeta{
				Name:      "cm-acme-http-solver-9m9mw",
				Namespace: "default",
				Labels: map[string]string{
					"acme.cert-manager.io/http01-solver": "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{
					Host: "ingress-to-create.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/.well-known/acme-challenge/0zdvVjgtDwEjCX6zIlynXvaP5Zekff4ZKQgezH_B4IM",
								PathType: &ptype,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "cm-acme-http-solver-7pf4j",
										Port: networkingv1.ServiceBackendPort{Number: 8089},
									},
									Resource: &v1.TypedLocalObjectReference{},
								},
							}},
						},
					},
				}},
			},
		},
		Services: map[types.NamespacedName]*v1.Service{
			{Name: "cm-acme-http-solver-7pf4j", Namespace: "default"}: {
				TypeMeta: v1meta.TypeMeta{},
				ObjectMeta: v1meta.ObjectMeta{
					Name:      "cm-acme-http-solver-7pf4j",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{{
						Name:        "http",
						Protocol:    "TCP",
						AppProtocol: new(string),
						Port:        8089,
						TargetPort:  intstr.FromInt(8089),
					}},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.True(t, routes[0].AllowPublicUnauthenticatedAccess)
	require.True(t, routes[0].PreserveHostHeader)
}

package pomerium

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/pomerium/ingress-controller/model"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func TestHttp01Solver(t *testing.T) {
	ptype := networkingv1.PathTypeExact
	routes, err := ingressToRoutes(context.Background(), &model.IngressConfig{
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
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
									Resource: &corev1.TypedLocalObjectReference{},
								},
							}},
						},
					},
				}},
			},
		},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "cm-acme-http-solver-7pf4j", Namespace: "default"}: {
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cm-acme-http-solver-7pf4j",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
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

func TestUpsertIngress(t *testing.T) {
	typePrefix := networkingv1.PathTypePrefix
	ic := &model.IngressConfig{
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"service.localhost.pomerium.io"},
					SecretName: "secret",
				}},
				Rules: []networkingv1.IngressRule{{
					Host: "service.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/a",
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
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "secret", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					corev1.TLSPrivateKeyKey: []byte("A"),
					corev1.TLSCertKey:       []byte("A"),
				},
				Type: corev1.SecretTypeTLS,
			}},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
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
		},
	}

	cfg := new(pb.Config)
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err := routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	require.NotNil(t, routes[routeID{
		Name:      "ingress",
		Namespace: "default",
		Path:      "/a",
		Host:      "service.localhost.pomerium.io",
	}])

	ic.Ingress.Spec.Rules[0].HTTP.Paths = append(ic.Ingress.Spec.Rules[0].HTTP.Paths, networkingv1.HTTPIngressPath{
		Path:     "/b",
		PathType: &typePrefix,
		Backend: networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: "service",
				Port: networkingv1.ServiceBackendPort{
					Name: "http",
				},
			},
		},
	})
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err = routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/a", Host: "service.localhost.pomerium.io"}])
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/b", Host: "service.localhost.pomerium.io"}])

	ic.Ingress.Spec.Rules[0].HTTP.Paths[0].Path = "/c"
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err = routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	require.Nil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/a", Host: "service.localhost.pomerium.io"}])
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/b", Host: "service.localhost.pomerium.io"}])
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/c", Host: "service.localhost.pomerium.io"}])
}

func TestSecureUpstream(t *testing.T) {
	typePrefix := networkingv1.PathTypePrefix
	ic := &model.IngressConfig{
		AnnotationPrefix: "p",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress",
				Namespace: "default",
				Annotations: map[string]string{
					fmt.Sprintf("p/%s", model.SecureUpstream): "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"service.localhost.pomerium.io"},
					SecretName: "secret",
				}},
				Rules: []networkingv1.IngressRule{{
					Host: "service.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/a",
								PathType: &typePrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "service",
										Port: networkingv1.ServiceBackendPort{
											Name: "https",
										},
									},
								},
							}},
						},
					},
				}},
			},
		},
		Endpoints: map[types.NamespacedName]*corev1.Endpoints{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{{
					Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}},
					Ports:     []corev1.EndpointPort{{Name: "https", Port: 443}},
				}},
			}},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "https",
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.IntOrString{IntVal: 443},
					}},
				},
				Status: corev1.ServiceStatus{},
			},
		},
	}

	cfg := new(pb.Config)
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err := routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	route := routes[routeID{
		Name:      "ingress",
		Namespace: "default",
		Path:      "/a",
		Host:      "service.localhost.pomerium.io",
	}]
	require.NotNil(t, route, "route not found in %v", routes)
	require.Equal(t, []string{
		"https://1.2.3.4:443",
	}, route.To)
}

func TestExternalService(t *testing.T) {
	makeRoute := func(t *testing.T, secure bool) (*pb.Route, error) {
		typePrefix := networkingv1.PathTypePrefix
		ic := &model.IngressConfig{
			AnnotationPrefix: "p",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
				},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{{
						Hosts:      []string{"service.localhost.pomerium.io"},
						SecretName: "secret",
					}},
					Rules: []networkingv1.IngressRule{{
						Host: "service.localhost.pomerium.io",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{
									Path:     "/a",
									PathType: &typePrefix,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "service",
											Port: networkingv1.ServiceBackendPort{
												Name: "app",
											},
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
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Type:         corev1.ServiceTypeExternalName,
						ExternalName: "service.external.com",
						Ports: []corev1.ServicePort{{
							Name:     "app",
							Protocol: "TCP",
							Port:     9999,
						}},
					},
				},
			},
		}
		if secure {
			ic.Ingress.Annotations = map[string]string{fmt.Sprintf("p/%s", model.SecureUpstream): "true"}
		}

		cfg := new(pb.Config)
		if err := upsertRoutes(context.Background(), cfg, ic); err != nil {
			return nil, fmt.Errorf("upsert routes: %w", err)
		}
		routes, err := routeList(cfg.Routes).toMap()
		if err != nil {
			return nil, err
		}
		return routes[routeID{
			Name:      "ingress",
			Namespace: "default",
			Path:      "/a",
			Host:      "service.localhost.pomerium.io",
		}], nil
	}

	for _, tc := range []struct {
		secure    bool
		expectURL string
	}{
		{
			secure:    false,
			expectURL: "http://service.external.com:9999",
		},
		{
			secure:    true,
			expectURL: "https://service.external.com:9999",
		},
	} {
		t.Run(fmt.Sprintf("%+v", tc), func(t *testing.T) {
			route, err := makeRoute(t, tc.secure)
			require.NoError(t, err)
			require.Equal(t, []string{tc.expectURL}, route.To)
		})
	}
}

func TestDefaultBackendService(t *testing.T) {
	typePrefix := networkingv1.PathTypePrefix
	typeExact := networkingv1.PathTypeExact
	icTemplate := func() *model.IngressConfig {
		return &model.IngressConfig{
			AnnotationPrefix: "p",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "default"},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{{
						Hosts:      []string{"service.localhost.pomerium.io"},
						SecretName: "secret",
					}},
					DefaultBackend: &networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: "service",
							Port: networkingv1.ServiceBackendPort{
								Name: "app",
							},
						},
					},
				},
			},
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "service", Namespace: "default"}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Type:         corev1.ServiceTypeExternalName,
						ExternalName: "service.external.com",
						Ports: []corev1.ServicePort{{
							Name:     "app",
							Protocol: "TCP",
							Port:     9999,
						}},
					},
				},
			},
		}
	}

	t.Run("just default backend", func(t *testing.T) {
		ic := icTemplate()
		cfg := new(pb.Config)
		t.Log(protojson.Format(cfg))
		require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
		require.Len(t, cfg.Routes, 1)
		assert.Equal(t, "/", cfg.Routes[0].Prefix)
	})

	t.Run("default backend and rule", func(t *testing.T) {
		ic := icTemplate()
		ic.Spec.Rules = []networkingv1.IngressRule{{
			Host: "service.localhost.pomerium.io",
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path:     "/two",
						PathType: &typeExact,
						Backend:  *ic.Spec.DefaultBackend,
					}, {
						Path:     "/one",
						PathType: &typePrefix,
						Backend:  *ic.Spec.DefaultBackend,
					}},
				},
			}}}
		cfg := new(pb.Config)
		require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
		sort.Sort(routeList(cfg.Routes))
		require.Len(t, cfg.Routes, 3)
		assert.Equal(t, "/", cfg.Routes[2].Prefix, protojson.Format(cfg))
	})
}

func TestRegexPath(t *testing.T) {
	pathType := networkingv1.PathTypeImplementationSpecific
	ic := &model.IngressConfig{
		AnnotationPrefix: "p",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress",
				Namespace: "default",
				Annotations: map[string]string{
					fmt.Sprintf("p/%s", model.PathRegex): "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"service.localhost.pomerium.io"},
					SecretName: "secret",
				}},
				Rules: []networkingv1.IngressRule{{
					Host: "service.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "^/(admin|superuser)/.*$",
								PathType: &pathType,
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
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
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
		},
	}

	cfg := new(pb.Config)
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err := routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	route := routes[routeID{
		Name:      "ingress",
		Namespace: "default",
		Path:      "^/(admin|superuser)/.*$",
		Host:      "service.localhost.pomerium.io",
	}]
	require.NotNil(t, route)
	assert.Equal(t, "^/(admin|superuser)/.*$", route.Regex, "regex")
	assert.Empty(t, route.Prefix, "prefix")
	assert.Empty(t, route.Path, "path")
}

func TestUseServiceProxy(t *testing.T) {
	pathTypePrefix := networkingv1.PathTypePrefix
	ic := &model.IngressConfig{
		AnnotationPrefix: "p",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress",
				Namespace: "default",
				Annotations: map[string]string{
					fmt.Sprintf("p/%s", model.UseServiceProxy): "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"service.localhost.pomerium.io"},
					SecretName: "secret",
				}},
				Rules: []networkingv1.IngressRule{{
					Host: "service.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/a",
								PathType: &pathTypePrefix,
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
		Endpoints: map[types.NamespacedName]*corev1.Endpoints{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{{
					Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}},
					Ports:     []corev1.EndpointPort{{Port: 80}},
				}},
			}},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
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
		},
	}

	cfg := new(pb.Config)
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err := routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	route := routes[routeID{
		Name:      "ingress",
		Namespace: "default",
		Path:      "/a",
		Host:      "service.localhost.pomerium.io",
	}]
	require.NotNil(t, route, "route not found in %v", routes)
	require.Equal(t, []string{
		"http://service.default.svc.cluster.local:80",
	}, route.To)
}

func TestSortRoutes(t *testing.T) {
	r1 := &pb.Route{
		Name: "route1",
		From: "http://a.example.com",
	}
	r2 := &pb.Route{
		Name: "route2",
		From: "http://b.example.com",
		Path: "/path/a",
	}
	r3 := &pb.Route{
		Name: "route3",
		From: "http://b.example.com",
		Path: "/path",
	}
	r4 := &pb.Route{
		Name:  "route4",
		From:  "http://b.example.com",
		Regex: "REGEX/A",
	}
	r5 := &pb.Route{
		Name:  "route5",
		From:  "http://b.example.com",
		Regex: "REGEX",
	}
	r6 := &pb.Route{
		Name:   "route6",
		From:   "http://b.example.com",
		Prefix: "/prefix/a/",
	}
	r7 := &pb.Route{
		Name:   "route7",
		From:   "http://b.example.com",
		Prefix: "/prefix/",
	}

	random := rand.New(rand.NewSource(0))

	for i := 0; i < 10; i++ {
		routes := routeList{r1, r2, r3, r4, r5, r6, r7}
		shuffleRoutes(random, routes)

		sort.Sort(routes)
		assert.Empty(t, cmp.Diff(routeList{r1, r2, r3, r4, r5, r6, r7}, routes, protocmp.Transform()))
	}
}

func shuffleRoutes(random *rand.Rand, routes []*pb.Route) {
	for i := range routes {
		j := random.Intn(i + 1)
		routes[i], routes[j] = routes[j], routes[i]
	}
}

// TestServicePortsAndEndpoints checks that only correct Endpoints would be selected for a Service
// https://github.com/pomerium/ingress-controller/issues/157
// - if there's just one port defined for the service, it may be defined in numerical form
// - if there are multiple, then name is required, that would be repeated in the endpoints
func TestServicePortsAndEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name            string
		ingressPort     networkingv1.ServiceBackendPort
		svcPorts        []corev1.ServicePort
		endpointSubsets []corev1.EndpointSubset
		expectTO        []string
		expectError     bool
	}{
		{
			"unnamed port",
			networkingv1.ServiceBackendPort{Number: 8080},
			[]corev1.ServicePort{{
				Port:       8080,
				TargetPort: intstr.IntOrString{IntVal: 80},
			}},
			[]corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}},
				Ports:     []corev1.EndpointPort{{Port: 80}},
			}},
			[]string{
				"http://1.2.3.4:80",
			},
			false,
		},
		{
			"named port",
			networkingv1.ServiceBackendPort{Name: "http"},
			[]corev1.ServicePort{{
				Name:       "http",
				Port:       8000,
				TargetPort: intstr.IntOrString{IntVal: 80},
			}},
			[]corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}},
				Ports:     []corev1.EndpointPort{{Name: "http", Port: 80}},
			}},
			[]string{
				"http://1.2.3.4:80",
			},
			false,
		},
		{
			"multiple IPs",
			networkingv1.ServiceBackendPort{Name: "http"},
			[]corev1.ServicePort{{
				Name:       "http",
				Port:       8000,
				TargetPort: intstr.IntOrString{IntVal: 80},
			}},
			[]corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}, {IP: "1.2.3.5"}},
				Ports:     []corev1.EndpointPort{{Name: "http", Port: 80}},
			}},
			[]string{
				"http://1.2.3.4:80",
				"http://1.2.3.5:80",
			},
			false,
		},
		{
			"multiple services",
			networkingv1.ServiceBackendPort{Name: "http"},
			[]corev1.ServicePort{{
				Name:       "http",
				Port:       8000,
				TargetPort: intstr.IntOrString{IntVal: 80},
			}, {
				Name:       "metrics",
				Port:       9090,
				TargetPort: intstr.IntOrString{IntVal: 8090},
			}},
			[]corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}, {IP: "1.2.3.5"}},
				Ports: []corev1.EndpointPort{
					{Name: "metrics", Port: 8090},
					{Name: "http", Port: 80},
				},
			}},
			[]string{
				"http://1.2.3.4:80",
				"http://1.2.3.5:80",
			},
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pathTypePrefix := networkingv1.PathTypePrefix
			ic := &model.IngressConfig{
				AnnotationPrefix: "p",
				Ingress: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ingress",
						Namespace: "default",
					},
					Spec: networkingv1.IngressSpec{
						TLS: []networkingv1.IngressTLS{{
							Hosts:      []string{"service.localhost.pomerium.io"},
							SecretName: "secret",
						}},
						Rules: []networkingv1.IngressRule{{
							Host: "service.localhost.pomerium.io",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{{
										Path:     "/a",
										PathType: &pathTypePrefix,
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: "service",
												Port: tc.ingressPort,
											},
										},
									}},
								},
							},
						}},
					},
				},
				Endpoints: map[types.NamespacedName]*corev1.Endpoints{
					{Name: "service", Namespace: "default"}: {
						ObjectMeta: metav1.ObjectMeta{
							Name:      "service",
							Namespace: "default",
						},
						Subsets: tc.endpointSubsets,
					}},
				Services: map[types.NamespacedName]*corev1.Service{
					{Name: "service", Namespace: "default"}: {
						ObjectMeta: metav1.ObjectMeta{
							Name:      "service",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							Ports: tc.svcPorts,
						},
						Status: corev1.ServiceStatus{},
					},
				},
			}

			cfg := new(pb.Config)
			require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
			routes, err := routeList(cfg.Routes).toMap()
			require.NoError(t, err)
			route := routes[routeID{
				Name:      "ingress",
				Namespace: "default",
				Path:      "/a",
				Host:      "service.localhost.pomerium.io",
			}]
			require.NotNil(t, route, "route not found in %v", routes)
			require.ElementsMatch(t, tc.expectTO, route.To)
			require.Empty(t, route.TlsServerName)
		})
	}
}

// TestEndpointsHTTPS verifies that endpoints would correctly set
// https://github.com/pomerium/ingress-controller/issues/164
func TestEndpointsHTTPS(t *testing.T) {
	for _, tc := range []struct {
		name                string
		ingressAnnotations  map[string]string
		ingressPort         networkingv1.ServiceBackendPort
		svcPorts            []corev1.ServicePort
		endpointSubsets     []corev1.EndpointSubset
		expectTO            []string
		expectTLSServerName string
	}{
		{
			"HTTPS multiple IPs - no annotations",
			map[string]string{
				fmt.Sprintf("p/%s", model.SecureUpstream): "true",
			},
			networkingv1.ServiceBackendPort{Name: "https"},
			[]corev1.ServicePort{{
				Name:       "https",
				Port:       443,
				TargetPort: intstr.IntOrString{IntVal: 443},
			}},
			[]corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}, {IP: "1.2.3.5"}},
				Ports:     []corev1.EndpointPort{{Name: "https", Port: 443}},
			}},
			[]string{
				"https://1.2.3.4:443",
				"https://1.2.3.5:443",
			},
			"service.default.svc.cluster.local",
		},
		{
			"HTTPS multiple IPs - override",
			map[string]string{
				fmt.Sprintf("p/%s", model.SecureUpstream): "true",
				fmt.Sprintf("p/%s", model.TLSServerName):  "custom-service.default.svc.cluster.local",
			},
			networkingv1.ServiceBackendPort{Name: "https"},
			[]corev1.ServicePort{{
				Name:       "https",
				Port:       443,
				TargetPort: intstr.IntOrString{IntVal: 443},
			}},
			[]corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}, {IP: "1.2.3.5"}},
				Ports:     []corev1.EndpointPort{{Name: "https", Port: 443}},
			}},
			[]string{
				"https://1.2.3.4:443",
				"https://1.2.3.5:443",
			},
			"custom-service.default.svc.cluster.local",
		},
		{
			"HTTPS use service proxy",
			map[string]string{
				fmt.Sprintf("p/%s", model.SecureUpstream):  "true",
				fmt.Sprintf("p/%s", model.UseServiceProxy): "true",
			},
			networkingv1.ServiceBackendPort{Name: "https"},
			[]corev1.ServicePort{{
				Name:       "https",
				Port:       443,
				TargetPort: intstr.IntOrString{IntVal: 443},
			}},
			[]corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}, {IP: "1.2.3.5"}},
				Ports:     []corev1.EndpointPort{{Name: "https", Port: 443}},
			}},
			[]string{
				"https://service.default.svc.cluster.local:443",
			},
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pathTypePrefix := networkingv1.PathTypePrefix
			ic := &model.IngressConfig{
				AnnotationPrefix: "p",
				Ingress: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "ingress",
						Namespace:   "default",
						Annotations: tc.ingressAnnotations,
					},
					Spec: networkingv1.IngressSpec{
						TLS: []networkingv1.IngressTLS{{
							Hosts:      []string{"service.localhost.pomerium.io"},
							SecretName: "secret",
						}},
						Rules: []networkingv1.IngressRule{{
							Host: "service.localhost.pomerium.io",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{{
										Path:     "/a",
										PathType: &pathTypePrefix,
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: "service",
												Port: tc.ingressPort,
											},
										},
									}},
								},
							},
						}},
					},
				},
				Endpoints: map[types.NamespacedName]*corev1.Endpoints{
					{Name: "service", Namespace: "default"}: {
						ObjectMeta: metav1.ObjectMeta{
							Name:      "service",
							Namespace: "default",
						},
						Subsets: tc.endpointSubsets,
					}},
				Services: map[types.NamespacedName]*corev1.Service{
					{Name: "service", Namespace: "default"}: {
						ObjectMeta: metav1.ObjectMeta{
							Name:      "service",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							Ports: tc.svcPorts,
						},
						Status: corev1.ServiceStatus{},
					},
				},
			}

			cfg := new(pb.Config)
			require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
			routes, err := routeList(cfg.Routes).toMap()
			require.NoError(t, err)
			route := routes[routeID{
				Name:      "ingress",
				Namespace: "default",
				Path:      "/a",
				Host:      "service.localhost.pomerium.io",
			}]
			require.NotNil(t, route, "route not found in %v", routes)
			require.ElementsMatch(t, tc.expectTO, route.To)
			require.Equal(t, tc.expectTLSServerName, route.TlsServerName)
		})
	}
}

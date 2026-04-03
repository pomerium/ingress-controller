package pomerium

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pomerium/sdk-go/proto/pomerium"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/model"
)

func setupReconciler(t *testing.T) (
	*controllers_mock.MockSDKClient,
	*controllers_mock.MockClient,
	*APIReconciler,
) {
	ctrl := gomock.NewController(t)
	apiClient := controllers_mock.NewMockSDKClient(ctrl)
	k8sClient := controllers_mock.NewMockClient(ctrl)

	r := &APIReconciler{
		apiClient:  apiClient,
		k8sClient:  k8sClient,
		secretsMap: model.NewTLSSecretsMap(),
	}
	return apiClient, k8sClient, r
}

func TestAPIReconcilerBasicIngressLifecycle(t *testing.T) {
	// Test the basics route and keypair lifecycle via the IngressReconciler methods.
	//   1. Create an Ingress without a TLS secret.
	//   2. Modify the Ingress to adjust the route details and add a TLS secret.
	//   3. Delete the Ingress.
	// APIReconciler should create, update, and delete a Pomerium route and
	// keypair entity.
	apiClient, k8sClient, r := setupReconciler(t)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: proto.String("pomerium"),
			Rules: []networkingv1.IngressRule{{
				Host: "a.localhost.pomerium.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							PathType: new(networkingv1.PathTypePrefix),
							Path:     "/",
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "example-svc",
									Port: networkingv1.ServiceBackendPort{
										Number: 8080,
									},
								},
							},
						}},
					},
				},
			}},
		},
	}
	ic := &model.IngressConfig{
		AnnotationPrefix: "a",
		Ingress:          ingress,
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "example-svc", Namespace: "test"}: {},
		},
	}

	ctx := t.Context()

	// Specifics around Secret and Ingress metadata updates will be tested in
	// other test cases.
	k8sClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// An initial call to Set() should create a Pomerium route from the Ingress.
	route := &pomerium.Route{
		OriginatorId: new("ingress-controller"),
		Name:         new("test-a-localhost-pomerium-io"),
		StatName:     new("test-a-localhost-pomerium-io"),
		From:         "https://a.localhost.pomerium.io",
		To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
		Prefix:       "/",
	}
	apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
		Route: route,
	})).Return(createRouteResponseWithID("new-route-id-1"), nil)

	changed, err := r.Set(t.Context(), []*model.IngressConfig{ic})
	assert.True(t, changed)
	require.NoError(t, err)

	// Modifying the Ingress should result in updates to the route and the
	// creation of a keypair entity.
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pomerium-wildcard-cert",
			Namespace: "test",
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("fake-cert-data"),
			"tls.key": []byte("fake-key-data"),
		},
	}
	ic.Spec.TLS = []networkingv1.IngressTLS{{
		SecretName: "pomerium-wildcard-cert",
	}}
	ic.Secrets = map[types.NamespacedName]*corev1.Secret{
		{Name: "pomerium-wildcard-cert", Namespace: "test"}: tlsSecret,
	}
	ic.Annotations["a/set_request_headers"] = `{"Foo": "bar"}`

	apiClient.EXPECT().CreateKeyPair(ctx, RequestEq(&pomerium.CreateKeyPairRequest{
		KeyPair: &pomerium.KeyPair{
			OriginatorId: new("ingress-controller"),
			Name:         new("test-pomerium-wildcard-cert"),
			Certificate:  []byte("fake-cert-data"),
			Key:          []byte("fake-key-data"),
		},
	})).Return(createKeyPairResponseWithID("new-keypair-id-1"), nil)
	apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
		Id: "new-route-id-1",
	})).Return(connect.NewResponse(&pomerium.GetRouteResponse{
		Route: route,
	}), nil)
	apiClient.EXPECT().UpdateRoute(ctx, RequestEq(&pomerium.UpdateRouteRequest{
		Route: &pomerium.Route{
			OriginatorId:      new("ingress-controller"),
			Id:                new("new-route-id-1"),
			Name:              new("test-a-localhost-pomerium-io"),
			StatName:          new("test-a-localhost-pomerium-io"),
			From:              "https://a.localhost.pomerium.io",
			To:                []string{"http://example-svc.test.svc.cluster.local:8080"},
			Prefix:            "/",
			SetRequestHeaders: map[string]string{"Foo": "bar"},
		},
	})).Return(connect.NewResponse(&pomerium.UpdateRouteResponse{}), nil)

	changed, err = r.Upsert(t.Context(), ic)
	assert.True(t, changed)
	require.NoError(t, err)

	// Deleting the Ingress should delete the route and keypair.
	k8sClient.EXPECT().Get(ctx, types.NamespacedName{
		Name: "pomerium-wildcard-cert", Namespace: "test",
	}, gomock.AssignableToTypeOf((*corev1.Secret)(nil))).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) error {
			(*dst).Annotations = map[string]string{
				"api.pomerium.io/keypair-id": "new-keypair-id-1",
			}
			return nil
		})
	apiClient.EXPECT().DeleteKeyPair(ctx, RequestEq(&pomerium.DeleteKeyPairRequest{
		Id: "new-keypair-id-1",
	})).Return(connect.NewResponse(&pomerium.DeleteKeyPairResponse{}), nil)
	apiClient.EXPECT().DeleteRoute(ctx, RequestEq(&pomerium.DeleteRouteRequest{
		Id: "new-route-id-1",
	})).Return(connect.NewResponse(&pomerium.DeleteRouteResponse{}), nil)

	changed, err = r.Delete(t.Context(), ic.GetIngressNamespacedName(), ic.Ingress)
	assert.True(t, changed)
	require.NoError(t, err)
}

func TestAPIReconciler_SetConfig(t *testing.T) {
	cfg := &model.Config{
		Pomerium: icsv1.Pomerium{
			Spec: icsv1.PomeriumSpec{
				Authenticate: &icsv1.Authenticate{
					URL: "https://authenticate.localhost.pomerium.io",
				},
				IdentityProvider: &icsv1.IdentityProvider{
					Provider: "oidc",
					URL:      proto.String("https://idp.example.com"),
					Secret:   "test/idp-client-secret",
				},
				PassIdentityHeaders: proto.Bool(true),
			},
		},
		IdpSecret: &corev1.Secret{
			Data: map[string][]byte{
				"client_id":     []byte("CLIENT_ID"),
				"client_secret": []byte("CLIENT_SECRET"),
			},
		},
	}

	t.Run("settings changed", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)

		// APIReconciler should first call GetSettings() to determine if it needs to sync any changes...
		apiClient.EXPECT().GetSettings(gomock.Any(), RequestEq(&pomerium.GetSettingsRequest{})).
			Return(&connect.Response[pomerium.GetSettingsResponse]{
				Msg: &pomerium.GetSettingsResponse{
					Settings: &pomerium.Settings{
						Id: proto.String("settings-id-123"),
					},
				},
			}, nil)

		// ...and then call UpdateSettings() once it knows there are changes to sync.
		apiClient.EXPECT().UpdateSettings(gomock.Any(), RequestEq(&pomerium.UpdateSettingsRequest{
			Settings: &pomerium.Settings{
				AuthenticateServiceUrl: proto.String("https://authenticate.localhost.pomerium.io"),
				IdpClientId:            proto.String("CLIENT_ID"),
				IdpClientSecret:        proto.String("CLIENT_SECRET"),
				IdpProvider:            proto.String("oidc"),
				IdpProviderUrl:         proto.String("https://idp.example.com"),
				PassIdentityHeaders:    proto.Bool(true),
			},
		})).Return(&connect.Response[pomerium.UpdateSettingsResponse]{
			Msg: &pomerium.UpdateSettingsResponse{},
		}, nil)

		changes, err := r.SetConfig(t.Context(), cfg)
		assert.True(t, changes)
		assert.NoError(t, err)
	})

	t.Run("settings unchanged", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)

		// If the settings already match, there should be no UpdateSettings() call.
		apiClient.EXPECT().GetSettings(gomock.Any(), connect.NewRequest(&pomerium.GetSettingsRequest{})).
			Return(&connect.Response[pomerium.GetSettingsResponse]{
				Msg: &pomerium.GetSettingsResponse{
					Settings: &pomerium.Settings{
						Id: proto.String("settings-id-123"),

						AuthenticateServiceUrl: proto.String("https://authenticate.localhost.pomerium.io"),
						IdpClientId:            proto.String("CLIENT_ID"),
						IdpClientSecret:        proto.String("CLIENT_SECRET"),
						IdpProvider:            proto.String("oidc"),
						IdpProviderUrl:         proto.String("https://idp.example.com"),
						PassIdentityHeaders:    proto.Bool(true)},
				},
			}, nil)

		changes, err := r.SetConfig(t.Context(), cfg)
		assert.False(t, changes)
		assert.NoError(t, err)
	})
}

func TestAPIReconciler_upsertOneKeyPair(t *testing.T) {
	secretTemplate := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-1",
			Namespace: "test",
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("cert-data"),
			corev1.TLSPrivateKeyKey: []byte("key-data"),
		},
	}

	t.Run("create new keypair", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		secret := secretTemplate.DeepCopy()

		// If there is no keypair ID annotation present, APIReconciler should
		// create a new keypair.
		apiClient.EXPECT().CreateKeyPair(gomock.Any(), RequestEq(&pomerium.CreateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: proto.String("ingress-controller"),
				Name:         proto.String("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(&connect.Response[pomerium.CreateKeyPairResponse]{
			Msg: &pomerium.CreateKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					Id: proto.String("new-keypair-id"),
					// rest of the data omitted (not currently read)
				},
			},
		}, nil)

		// APIReconciler should make a Patch() request to record the
		// newly-assigned ID in the keypair ID annotation (verified below).
		k8sClient.EXPECT().Patch(gomock.Any(), secret, gomock.Any()).Return(nil)

		changed, err := r.upsertOneKeyPair(t.Context(), secret)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Equal(t, "new-keypair-id", secret.Annotations[apiKeyPairIDAnnotation])
		assert.Contains(t, secret.Finalizers, apiFinalizer)
	})

	t.Run("update existing keypair", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		secret := secretTemplate.DeepCopy()
		secret.Annotations = map[string]string{
			apiKeyPairIDAnnotation: "existing-keypair-id",
		}

		// APIReconciler should first call GetKeyPair() to determine if it needs to sync any changes...
		apiClient.EXPECT().GetKeyPair(gomock.Any(), RequestEq(&pomerium.GetKeyPairRequest{
			Id: "existing-keypair-id",
		})).Return(&connect.Response[pomerium.GetKeyPairResponse]{
			Msg: &pomerium.GetKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					OriginatorId: proto.String("ingress-controller"),
					Id:           proto.String("existing-keypair-id"),
					Name:         proto.String("test-secret-1"),
					Certificate:  []byte("different-cert-data"),
					Key:          []byte("different-key-data"),
				},
			},
		}, nil)

		// ...and then UpdateKeyPair() to sync changes.
		apiClient.EXPECT().UpdateKeyPair(gomock.Any(), RequestEq(&pomerium.UpdateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: proto.String("ingress-controller"),
				Id:           proto.String("existing-keypair-id"),
				Name:         proto.String("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(&connect.Response[pomerium.UpdateKeyPairResponse]{
			Msg: &pomerium.UpdateKeyPairResponse{},
		}, nil)

		changed, err := r.upsertOneKeyPair(t.Context(), secret)
		assert.True(t, changed)
		assert.NoError(t, err)
	})

	t.Run("existing keypair unchanged", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		secret := secretTemplate.DeepCopy()
		secret.Annotations = map[string]string{
			apiKeyPairIDAnnotation: "existing-keypair-id",
		}

		// If the keypair already matches, there should be no UpdateKeyPair() call.
		apiClient.EXPECT().GetKeyPair(gomock.Any(), RequestEq(&pomerium.GetKeyPairRequest{
			Id: "existing-keypair-id",
		})).Return(&connect.Response[pomerium.GetKeyPairResponse]{
			Msg: &pomerium.GetKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					OriginatorId: proto.String("ingress-controller"),
					Id:           proto.String("existing-keypair-id"),
					Name:         proto.String("test-secret-1"),
					Certificate:  []byte("cert-data"),
					Key:          []byte("key-data"),

					// these fields should be ignored
					CertificateInfo: []*pomerium.CertificateInfo{{
						Version: 1234,
						Serial:  "ABCD",
					}},
					Status: pomerium.KeyPairStatus_KEY_PAIR_STATUS_READY,
					Origin: pomerium.KeyPairOrigin_KEY_PAIR_ORIGIN_USER,
				},
			},
		}, nil)

		changed, err := r.upsertOneKeyPair(t.Context(), secret)
		assert.False(t, changed)
		assert.NoError(t, err)
	})

	t.Run("existing keypair not found", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		secret := secretTemplate.DeepCopy()
		secret.Annotations = map[string]string{
			apiKeyPairIDAnnotation: "existing-keypair-id",
		}

		// If there is an existing keypair ID annotation present, but it cannot
		// be retrieved, APIReconciler should create it as a new keypair.
		apiClient.EXPECT().GetKeyPair(gomock.Any(), RequestEq(&pomerium.GetKeyPairRequest{
			Id: "existing-keypair-id",
		})).Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))

		apiClient.EXPECT().CreateKeyPair(gomock.Any(), RequestEq(&pomerium.CreateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				Id:           proto.String("existing-keypair-id"),
				OriginatorId: proto.String("ingress-controller"),
				Name:         proto.String("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(&connect.Response[pomerium.CreateKeyPairResponse]{
			Msg: &pomerium.CreateKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					Id: proto.String("existing-keypair-id"),
					// rest of the data omitted (not currently read)
				},
			},
		}, nil)

		changed, err := r.upsertOneKeyPair(t.Context(), secret)
		assert.True(t, changed)
		assert.NoError(t, err)
	})
}

func TestAPIReconciler_upsertOneIngress(t *testing.T) {
	irv := networkingv1.IngressRuleValue{
		HTTP: &networkingv1.HTTPIngressRuleValue{
			Paths: []networkingv1.HTTPIngressPath{{
				PathType: new(networkingv1.PathTypePrefix),
				Path:     "/",
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: "example-svc",
						Port: networkingv1.ServiceBackendPort{
							Number: 8080,
						},
					},
				},
			}},
		},
	}
	ingressTemplate := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: proto.String("pomerium"),
			Rules: []networkingv1.IngressRule{
				{
					Host:             "a.localhost.pomerium.io",
					IngressRuleValue: irv,
				},
				{
					Host:             "b.localhost.pomerium.io",
					IngressRuleValue: irv,
				},
			},
		},
	}
	services := map[types.NamespacedName]*corev1.Service{
		{Name: "example-svc", Namespace: "test"}: {},
	}

	t.Run("create new routes", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ingress := ingressTemplate.DeepCopy()
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress:          ingress,
			Services:         services,
		}

		// If there are no existing route ID anotations, APIReconciler should
		// create new routes.
		apiClient.EXPECT().CreateRoute(gomock.Any(), RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-a-localhost-pomerium-io"),
				StatName:     new("test-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		})).Return(createRouteResponseWithID("new-route-id-1"), nil)
		apiClient.EXPECT().CreateRoute(gomock.Any(), RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-b-localhost-pomerium-io"),
				StatName:     new("test-b-localhost-pomerium-io"),
				From:         "https://b.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		})).Return(createRouteResponseWithID("new-route-id-2"), nil)

		// APIReconciler should make a Patch() request to record the
		// new route ID annotations (verified below).
		k8sClient.EXPECT().Patch(gomock.Any(), ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(t.Context(), ic)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Len(t, allRouteIDAnnotations(ic.Annotations), 2)
		assert.Equal(t, "new-route-id-1", ic.Annotations["api.pomerium.io/route-id-0"])
		assert.Equal(t, "new-route-id-2", ic.Annotations["api.pomerium.io/route-id-1"])
		assert.Contains(t, ic.Finalizers, apiFinalizer)
	})

	/*
		t.Run("update existing route", func(t *testing.T) {
			apiClient.EXPECT().GetRoute(gomock.Any(), RequestEq(&pomerium.GetRouteRequest{
				Id: "existing-route-id",
			})).Return(&connect.Response[pomerium.GetRouteResponse]{
				Msg: &pomerium.GetRouteResponse{
					Route: &pomerium.Route{
						Id:   proto.String("existing-route-id"),
						Name: proto.String("test-route"),
						From: proto.String("https://test.example.com"),
						To:   []string{"http://old-backend:8080"},
					},
				},
			}, nil)

			apiClient.EXPECT().UpdateRoute(gomock.Any(), gomock.Any()).
				Return(&connect.Response[pomerium.UpdateRouteResponse]{
					Msg: &pomerium.UpdateRouteResponse{},
				}, nil)

			changed, err := r.upsertOneRoute(t.Context(), "existing-route-id", route)
			assert.True(t, changed)
			assert.NoError(t, err)
		})

		t.Run("existing route unchanged", func(t *testing.T) {
			apiClient.EXPECT().GetRoute(gomock.Any(), RequestEq(&pomerium.GetRouteRequest{
				Id: "existing-route-id",
			})).Return(&connect.Response[pomerium.GetRouteResponse]{
				Msg: &pomerium.GetRouteResponse{
					Route: &pomerium.Route{
						Id:           proto.String("existing-route-id"),
						Name:         proto.String("test-route"),
						From:         proto.String("https://test.example.com"),
						To:           []string{"http://backend:8080"},
						OriginatorId: &originatorID,
					},
				},
			}, nil)

			// No UpdateRoute call expected since nothing changed.

			changed, err := r.upsertOneRoute(t.Context(), "existing-route-id", route)
			assert.False(t, changed)
			assert.NoError(t, err)
		})
	*/
}

/*
func TestAPIReconciler_deleteRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	apiClient := controllers_mock.NewMockSDKClient(ctrl)

	r := APIReconciler{
		apiClient:  apiClient,
		secretsMap: model.NewTLSSecretsMap(),
	}

	t.Run("delete routes", func(t *testing.T) {
		ingress := &networkingv1.Ingress{}
		ingress.Annotations = map[string]string{
			"api.pomerium.io/route-id-0": "route-id-0",
			"api.pomerium.io/route-id-1": "route-id-1",
		}

		routeAnnotations := map[string]struct{}{
			"api.pomerium.io/route-id-0": {},
			"api.pomerium.io/route-id-1": {},
		}

		apiClient.EXPECT().DeleteRoute(gomock.Any(), RequestEq(&pomerium.DeleteRouteRequest{
			Id: "route-id-0",
		})).Return(&connect.Response[pomerium.DeleteRouteResponse]{}, nil)

		apiClient.EXPECT().DeleteRoute(gomock.Any(), RequestEq(&pomerium.DeleteRouteRequest{
			Id: "route-id-1",
		})).Return(&connect.Response[pomerium.DeleteRouteResponse]{}, nil)

		anyDeletes, err := r.deleteRoutes(t.Context(), ingress, routeAnnotations)
		assert.True(t, anyDeletes)
		assert.NoError(t, err)
		assert.Empty(t, ingress.Annotations)
	})

	t.Run("delete routes ignores not found", func(t *testing.T) {
		ingress := &networkingv1.Ingress{}
		ingress.Annotations = map[string]string{
			"api.pomerium.io/route-id-0": "route-id-0",
		}

		routeAnnotations := map[string]struct{}{
			"api.pomerium.io/route-id-0": {},
		}

		apiClient.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).
			Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))

		anyDeletes, err := r.deleteRoutes(t.Context(), ingress, routeAnnotations)
		assert.True(t, anyDeletes)
		assert.NoError(t, err)
	})
}

func TestAPIReconciler_deletePolicy(t *testing.T) {
	ctrl := gomock.NewController(t)
	apiClient := controllers_mock.NewMockSDKClient(ctrl)

	r := APIReconciler{
		apiClient:  apiClient,
		secretsMap: model.NewTLSSecretsMap(),
	}

	t.Run("delete policy", func(t *testing.T) {
		ingress := &networkingv1.Ingress{}
		ingress.Annotations = map[string]string{
			apiPolicyIDAnnotation: "policy-id-123",
		}

		apiClient.EXPECT().DeletePolicy(gomock.Any(), RequestEq(&pomerium.DeletePolicyRequest{
			Id: "policy-id-123",
		})).Return(&connect.Response[pomerium.DeletePolicyResponse]{}, nil)

		deleted, err := r.deletePolicy(t.Context(), ingress)
		assert.True(t, deleted)
		assert.NoError(t, err)
		assert.Empty(t, ingress.Annotations[apiPolicyIDAnnotation])
	})

	t.Run("delete policy no-op when no annotation", func(t *testing.T) {
		ingress := &networkingv1.Ingress{}

		// No DeletePolicy call expected.

		deleted, err := r.deletePolicy(t.Context(), ingress)
		assert.False(t, deleted)
		assert.NoError(t, err)
	})

	t.Run("delete policy ignores not found", func(t *testing.T) {
		ingress := &networkingv1.Ingress{}
		ingress.Annotations = map[string]string{
			apiPolicyIDAnnotation: "policy-id-123",
		}

		apiClient.EXPECT().DeletePolicy(gomock.Any(), gomock.Any()).
			Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))

		deleted, err := r.deletePolicy(t.Context(), ingress)
		assert.True(t, deleted)
		assert.NoError(t, err)
	})
}*/

// XXX: We don't actually need to test this directly, do we?
func TestAllRouteIDAnnotations(t *testing.T) {
	annotations := map[string]string{
		"api.pomerium.io/route-id-0": "route-0",
		"api.pomerium.io/route-id-1": "route-1",
		"api.pomerium.io/route-id-2": "route-2",
		"api.pomerium.io/policy-id":  "policy-id",
		"other-annotation":           "value",
	}

	result := allRouteIDAnnotations(annotations)
	assert.ElementsMatch(t, []string{
		"api.pomerium.io/route-id-0",
		"api.pomerium.io/route-id-1",
		"api.pomerium.io/route-id-2",
	}, slices.Collect(maps.Keys(result)))
}

func TestRouteIDAnnotationForIndex(t *testing.T) {
	assert.Equal(t, "api.pomerium.io/route-id-0", routeIDAnnotationForIndex(0))
	assert.Equal(t, "api.pomerium.io/route-id-1", routeIDAnnotationForIndex(1))
	assert.Equal(t, "api.pomerium.io/route-id-99", routeIDAnnotationForIndex(99))
}

func createKeyPairResponseWithID(id string) *connect.Response[pomerium.CreateKeyPairResponse] {
	return &connect.Response[pomerium.CreateKeyPairResponse]{
		Msg: &pomerium.CreateKeyPairResponse{
			KeyPair: &pomerium.KeyPair{
				Id: &id,
				// the rest of the keypair data is not currently read
			},
		},
	}
}

func createRouteResponseWithID(id string) *connect.Response[pomerium.CreateRouteResponse] {
	return &connect.Response[pomerium.CreateRouteResponse]{
		Msg: &pomerium.CreateRouteResponse{
			Route: &pomerium.Route{
				Id: &id,
				// the rest of the route data is not currently read
			},
		},
	}
}

type requestMatcher[T any, P interface {
	proto.Message
	*T
}] struct {
	expected P
}

func RequestEq[T any, P interface {
	proto.Message
	*T
}](expected P) gomock.Matcher {
	return requestMatcher[T, P]{expected: expected}
}

func (m requestMatcher[T, P]) Matches(x interface{}) bool {
	req, ok := x.(*connect.Request[T])
	if !ok {
		return false
	}
	return proto.Equal(m.expected, P(req.Msg))
}

func (m requestMatcher[T, P]) String() string {
	return fmt.Sprintf("request with msg %[1]v (%[1]T)", m.expected)
}

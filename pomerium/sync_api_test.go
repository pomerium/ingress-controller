package pomerium

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/sdk-go/proto/pomerium"

	icgv1alpha1 "github.com/pomerium/ingress-controller/apis/gateway/v1alpha1"
	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium/gateway"
)

var exampleIngressRuleValue = networkingv1.IngressRuleValue{
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
		secretsMap: model.NewTLSSecretsMap(),
	}
	r.SetK8sClient(k8sClient)
	return apiClient, k8sClient, r
}

func TestAPIReconcilerBasicIngressLifecycle(t *testing.T) {
	// Test the basic route and keypair lifecycle via the IngressReconciler methods.
	//   1. Create an Ingress without a TLS secret.
	//   2. Modify the Ingress to adjust the route details and add a TLS secret.
	//   3. Delete the Ingress.
	// APIReconciler should create, update, and delete a Pomerium route and
	// keypair entity.
	apiClient, k8sClient, r := setupReconciler(t)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress",
			Namespace: "test",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: new("pomerium"),
			Rules: []networkingv1.IngressRule{{
				Host:             "a.localhost.pomerium.io",
				IngressRuleValue: exampleIngressRuleValue,
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
		Name:         new("test-my-ingress-a-localhost-pomerium-io"),
		From:         "https://a.localhost.pomerium.io",
		To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
		Prefix:       "/",
	}
	apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
		Route: route,
	})).Return(createRouteResponseWithID("new-route-id-1"), nil)

	changed, err := r.Set(ctx, []*model.IngressConfig{ic})
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
			Name:              new("test-my-ingress-a-localhost-pomerium-io"),
			From:              "https://a.localhost.pomerium.io",
			To:                []string{"http://example-svc.test.svc.cluster.local:8080"},
			Prefix:            "/",
			SetRequestHeaders: map[string]string{"Foo": "bar"},
		},
	})).Return(connect.NewResponse(&pomerium.UpdateRouteResponse{}), nil)

	changed, err = r.Upsert(ctx, ic)
	assert.True(t, changed)
	require.NoError(t, err)

	// Deleting the Ingress should delete the route and keypair.
	k8sClient.EXPECT().Get(ctx, types.NamespacedName{
		Name: "my-ingress", Namespace: "test",
	}, gomock.AssignableToTypeOf((*networkingv1.Ingress)(nil))).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, dst *networkingv1.Ingress, _ ...client.GetOption) error {
			*dst = *ic.Ingress
			return nil
		})
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

	changed, err = r.Delete(ctx, ic.GetIngressNamespacedName())
	assert.True(t, changed)
	require.NoError(t, err)
}

func TestAPIReconciler_Delete(t *testing.T) {
	apiClient, k8sClient, r := setupReconciler(t)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress",
			Namespace: "test",
			Annotations: map[string]string{
				"api.pomerium.io/route-id-0": "existing-route-id",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: new("pomerium"),
			Rules: []networkingv1.IngressRule{{
				Host:             "a.localhost.pomerium.io",
				IngressRuleValue: exampleIngressRuleValue,
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

	k8sClient.EXPECT().Get(ctx, types.NamespacedName{
		Name: "my-ingress", Namespace: "test",
	}, gomock.AssignableToTypeOf((*networkingv1.Ingress)(nil))).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, dst *networkingv1.Ingress, _ ...client.GetOption) error {
			*dst = *ic.Ingress
			return nil
		})

	apiClient.EXPECT().DeleteRoute(ctx, RequestEq(&pomerium.DeleteRouteRequest{
		Id: "existing-route-id",
	})).Return(connect.NewResponse(&pomerium.DeleteRouteResponse{}), nil)

	// If the metadata patch operation fails, this error should be surfaced.
	patchErr := fmt.Errorf("failed to patch")
	k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(patchErr)

	changed, err := r.Delete(ctx, types.NamespacedName{Name: "my-ingress", Namespace: "test"})
	assert.True(t, changed)
	assert.ErrorIs(t, err, patchErr)
}

func TestAPIReconciler_upsertOneIngress(t *testing.T) {
	ingressTemplate := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress",
			Namespace: "test",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: new("pomerium"),
			Rules: []networkingv1.IngressRule{{
				Host:             "a.localhost.pomerium.io",
				IngressRuleValue: exampleIngressRuleValue,
			}},
		},
	}

	t.Run("create policy", func(t *testing.T) {
		// Verify that policy annotations translate to a standalone Policy entity.
		ppl := `allow:
  or:
    - groups:
        has: "engineering"`
		ingress := ingressTemplate.DeepCopy()
		ingress.Annotations = map[string]string{
			"a/policy": ppl,
		}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress:          ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		apiClient.EXPECT().CreatePolicy(ctx, RequestEq(&pomerium.CreatePolicyRequest{
			Policy: &pomerium.Policy{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-policy"),
				SourcePpl:    new(`[{"allow":{"or":[{"groups":{"has":"engineering"}}]}}]`),
			},
		})).Return(createPolicyResponseWithID("new-policy-id"), nil)
		apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",

				PolicyIds: []string{"new-policy-id"},
			},
		})).Return(createRouteResponseWithID("new-route-id"), nil)

		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Equal(t, "new-policy-id", ic.Annotations["api.pomerium.io/policy-id"])
		assert.Equal(t, "new-route-id", ic.Annotations["api.pomerium.io/route-id-0"])
		assert.Contains(t, ic.Finalizers, apiFinalizer)
	})

	t.Run("update policy", func(t *testing.T) {
		// Verify that a changed policy will be updated via the API.
		ppl := `allow:
  or:
    - groups:
        has: "admins"`
		ingress := ingressTemplate.DeepCopy()
		ingress.Annotations = map[string]string{
			"a/policy": ppl,
			// this route + policy have already been synced via the API
			"api.pomerium.io/policy-id":  "existing-policy-id",
			"api.pomerium.io/route-id-0": "existing-route-id",
		}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress:          ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		// This GetPolicy() response indicates the policy has changed.
		apiClient.EXPECT().GetPolicy(ctx, RequestEq(&pomerium.GetPolicyRequest{
			Id: "existing-policy-id",
		})).Return(connect.NewResponse(&pomerium.GetPolicyResponse{
			Policy: &pomerium.Policy{
				Id:           new("existing-policy-id"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-policy"),
				SourcePpl:    new(`[{"allow":{"or":[{"groups":{"has":"engineering"}}]}}]`),
			},
		}), nil)
		// This should result in an UpdatePolicy() call to sync the policy.
		apiClient.EXPECT().UpdatePolicy(ctx, RequestEq(&pomerium.UpdatePolicyRequest{
			Policy: &pomerium.Policy{
				Id:           new("existing-policy-id"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-policy"),
				SourcePpl:    new(`[{"allow":{"or":[{"groups":{"has":"admins"}}]}}]`),
			},
		})).Return(connect.NewResponse(&pomerium.UpdatePolicyResponse{}), nil)

		// This GetRoute() response indicates the route has not changed, so no
		// UpdateRoute() call is needed.
		apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
			Id: "existing-route-id",
		})).Return(connect.NewResponse(&pomerium.GetRouteResponse{
			Route: &pomerium.Route{
				Id:           new("existing-route-id"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				StatName:     new("stat-name-should-be-ignored"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
				PolicyIds:    []string{"existing-policy-id"},
			},
		}), nil)

		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		require.NoError(t, err)
	})

	t.Run("delete policy", func(t *testing.T) {
		// If a previous policy is no longer needed, it should be deleted.
		ingress := ingressTemplate.DeepCopy()
		ingress.Annotations = map[string]string{
			"api.pomerium.io/route-id-0": "existing-route-id",
			// Note: no policy rule annotations here, only a previous policy ID
			"api.pomerium.io/policy-id": "existing-policy-id",
		}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress:          ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
			Id: "existing-route-id",
		})).Return(connect.NewResponse(&pomerium.GetRouteResponse{
			Route: &pomerium.Route{
				Id:           new("existing-route-id"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				StatName:     new("route-stat-name"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",

				PolicyIds: []string{"existing-policy-id"},
			},
		}), nil)
		apiClient.EXPECT().UpdateRoute(ctx, RequestEq(&pomerium.UpdateRouteRequest{
			Route: &pomerium.Route{
				Id:           new("existing-route-id"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",

				PolicyIds: nil,
			},
		})).Return(connect.NewResponse(&pomerium.UpdateRouteResponse{}), nil)
		apiClient.EXPECT().DeletePolicy(ctx, RequestEq(&pomerium.DeletePolicyRequest{
			Id: "existing-policy-id",
		})).Return(connect.NewResponse(&pomerium.DeletePolicyResponse{}), nil)

		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.NotContains(t, ic.Annotations, "api.pomerium.io/policy-id")
	})

	t.Run("TLS secrets", func(t *testing.T) {
		// Verify that the custom TLS annotations translate to keypair ID references.
		ingress := ingressTemplate.DeepCopy()
		ingress.Annotations = map[string]string{
			"a/tls_custom_ca_secret":            "my-ca-cert",
			"a/tls_client_secret":               "my-client-cert",
			"a/tls_downstream_client_ca_secret": "my-downstream-client-ca-cert",
		}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress:          ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
			Secrets: map[types.NamespacedName]*corev1.Secret{
				{Name: "my-ca-cert", Namespace: "test"}: {
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"api.pomerium.io/keypair-id": "ca-cert-id",
						},
					},
					Data: map[string][]byte{
						"ca.crt": []byte("fake-cert-data-1"),
					},
				},
				{Name: "my-client-cert", Namespace: "test"}: {
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"api.pomerium.io/keypair-id": "client-cert-id",
						},
					},
					Data: map[string][]byte{
						"tls.crt": []byte("fake-cert-data-2"),
						"tls.key": []byte("fake-key-data"),
					},
				},
				{Name: "my-downstream-client-ca-cert", Namespace: "test"}: {
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"api.pomerium.io/keypair-id": "downstream-client-ca-cert-id",
						},
					},
					Data: map[string][]byte{
						"ca.crt": []byte("fake-cert-data-3"),
					},
				},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",

				TlsCustomCaKeyPairId:           new("ca-cert-id"),
				TlsClientKeyPairId:             new("client-cert-id"),
				TlsDownstreamClientCaKeyPairId: new("downstream-client-ca-cert-id"),
			},
		})).Return(createRouteResponseWithID("new-route-id"), nil)

		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Equal(t, "new-route-id", ic.Annotations["api.pomerium.io/route-id-0"])
		assert.Contains(t, ic.Finalizers, apiFinalizer)
	})

	t.Run("not found error on update", func(t *testing.T) {
		ingress := ingressTemplate.DeepCopy()
		ingress.Annotations = map[string]string{
			"api.pomerium.io/route-id-0": "existing-route-id",
		}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a", Ingress: ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		// If the GetRoute() call returns a Not Found error, the route should be
		// recreated using CreateRoute().
		apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
			Id: "existing-route-id",
		})).Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))
		apiClient.EXPECT().CreateRoute(ctx, gomock.Any()).
			Return(createRouteResponseWithID("recreated-route-id"), nil)

		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		assert.NoError(t, err)
		assert.Equal(t, "recreated-route-id", ic.Annotations["api.pomerium.io/route-id-0"])
	})

	t.Run("other error on update", func(t *testing.T) {
		ingress := ingressTemplate.DeepCopy()
		ingress.Annotations = map[string]string{
			"api.pomerium.io/route-id-0": "existing-route-id",
		}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a", Ingress: ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		// If the GetRoute() call returns some error other than Not Found, it
		// should propagate back in the return parameter.
		apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
			Id: "existing-route-id",
		})).Return(nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("unavailable")))
		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		_, err := r.upsertOneIngress(ctx, ic)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	})

	t.Run("already exists", func(t *testing.T) {
		ingress := ingressTemplate.DeepCopy()
		ic := &model.IngressConfig{
			AnnotationPrefix: "a", Ingress: ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		// If we try to create a route but it already exists, we should attempt
		// to look up the existing route by name.
		apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		})).Return(nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("already exists")))

		apiClient.EXPECT().ListRoutes(ctx, RequestEq(&pomerium.ListRoutesRequest{
			Filter: filterByName(t, "test-my-ingress-a-localhost-pomerium-io"),
		})).Return(connect.NewResponse(&pomerium.ListRoutesResponse{
			Routes: []*pomerium.Route{{
				Id:           new("missing-route-id"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			}},
		}), nil)

		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		assert.NoError(t, err)
		assert.Equal(t, "missing-route-id", ic.Annotations["api.pomerium.io/route-id-0"])
	})

	t.Run("patch error", func(t *testing.T) {
		ingress := ingressTemplate.DeepCopy()
		ic := &model.IngressConfig{
			AnnotationPrefix: "a", Ingress: ingress,
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}

		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-my-ingress-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		})).Return(createRouteResponseWithID("new-route-id"), nil)

		// If the metadata patch operation fails, this error should be surfaced.
		patchErr := fmt.Errorf("failed to patch")
		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(patchErr)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		assert.ErrorIs(t, err, patchErr)
	})
}

func TestAPIReconciler_upsertOneIngress_multipleRoutes(t *testing.T) {
	// Verify that APIReconciler will correctly sync an Ingress with multiple
	// rules, updating the route ID annotations as expected.

	ingressTemplate := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: new("pomerium"),
			Rules: []networkingv1.IngressRule{
				{
					Host:             "a.localhost.pomerium.io",
					IngressRuleValue: exampleIngressRuleValue,
				},
				{
					Host:             "b.localhost.pomerium.io",
					IngressRuleValue: exampleIngressRuleValue,
				},
			},
		},
	}
	services := map[types.NamespacedName]*corev1.Service{
		{Name: "example-svc", Namespace: "test"}: {},
	}

	t.Run("create multiple routes", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		ingress := ingressTemplate.DeepCopy()
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress:          ingress,
			Services:         services,
		}

		// If there are no existing route ID annotations, APIReconciler should
		// create new routes.
		apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-a-localhost-pomerium-io"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		})).Return(createRouteResponseWithID("new-route-id-A"), nil)
		apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
			Route: &pomerium.Route{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-b-localhost-pomerium-io"),
				From:         "https://b.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		})).Return(createRouteResponseWithID("new-route-id-B"), nil)

		// APIReconciler should make a Patch() request to record the
		// new route ID annotations (verified below).
		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Len(t, allRouteIDAnnotations(ic.Annotations), 2)
		assert.Equal(t, "new-route-id-A", ic.Annotations["api.pomerium.io/route-id-0"])
		assert.Equal(t, "new-route-id-B", ic.Annotations["api.pomerium.io/route-id-1"])
		assert.Contains(t, ic.Finalizers, apiFinalizer)
	})

	t.Run("deleted unneeded routes", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		ingress := ingressTemplate.DeepCopy()
		ingress.Annotations = map[string]string{
			"api.pomerium.io/route-id-0": "route-id-A",
			"api.pomerium.io/route-id-1": "route-id-B",
			"api.pomerium.io/route-id-2": "route-id-C",
			"api.pomerium.io/route-id-3": "route-id-D",
		}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress:          ingress,
			Services:         services,
		}

		apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
			Id: "route-id-A",
		})).Return(connect.NewResponse(&pomerium.GetRouteResponse{
			Route: &pomerium.Route{
				Id:           new("route-id-A"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-a-localhost-pomerium-io"),
				StatName:     new("route-a-stat-name"),
				From:         "https://a.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		}), nil)
		apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
			Id: "route-id-B",
		})).Return(connect.NewResponse(&pomerium.GetRouteResponse{
			Route: &pomerium.Route{
				Id:           new("route-id-B"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-b-localhost-pomerium-io"),
				StatName:     new("route-b-stat-name"),
				From:         "https://b.localhost.pomerium.io",
				To:           []string{"http://example-svc.test.svc.cluster.local:8080"},
				Prefix:       "/",
			},
		}), nil)

		// If there are more route ID annotations than currently-needed routes,
		// the unnecessary routes should be deleted.
		apiClient.EXPECT().DeleteRoute(ctx, RequestEq(&pomerium.DeleteRouteRequest{
			Id: "route-id-C",
		})).Return(connect.NewResponse(&pomerium.DeleteRouteResponse{}), nil)
		apiClient.EXPECT().DeleteRoute(ctx, RequestEq(&pomerium.DeleteRouteRequest{
			Id: "route-id-D",
		})).Return(connect.NewResponse(&pomerium.DeleteRouteResponse{}), nil)

		// APIReconciler should make a Patch() request to remove the ID
		// annotations for the deleted routes.
		k8sClient.EXPECT().Patch(ctx, ingress, gomock.Any()).Return(nil)

		changed, err := r.upsertOneIngress(ctx, ic)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Len(t, allRouteIDAnnotations(ic.Annotations), 2)
		assert.Equal(t, "route-id-A", ic.Annotations["api.pomerium.io/route-id-0"])
		assert.Equal(t, "route-id-B", ic.Annotations["api.pomerium.io/route-id-1"])
	})
}

func TestAPIReconciler_SetGatewayConfig(t *testing.T) {
	// Test the basic route & policy lifecycle via the SetGatewayConfig method.
	apiClient, k8sClient, r := setupReconciler(t)
	ctx := t.Context()

	examplePPL := `allow:
  or:
    - email:
        is: "me@example.com"`
	policyFilterObject := &icgv1alpha1.PolicyFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-policy",
			Namespace: "test",
		},
		Spec: icgv1alpha1.PolicyFilterSpec{
			PPL: examplePPL,
		},
	}
	examplePolicyFilter, err := gateway.NewPolicyFilter(policyFilterObject)
	require.NoError(t, err)

	httpRouteObject := &gateway_v1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-a",
			Namespace: "test",
		},
		Spec: gateway_v1.HTTPRouteSpec{
			CommonRouteSpec: gateway_v1.CommonRouteSpec{},
			Hostnames:       []gateway_v1.Hostname{},
			Rules: []gateway_v1.HTTPRouteRule{{
				Filters: []gateway_v1.HTTPRouteFilter{{
					Type: "ExtensionRef",
					ExtensionRef: &gateway_v1.LocalObjectReference{
						Group: "gateway.pomerium.io",
						Kind:  "PolicyFilter",
						Name:  "example-policy",
					},
				}},
				BackendRefs: []gateway_v1.HTTPBackendRef{{
					BackendRef: gateway_v1.BackendRef{
						BackendObjectReference: gateway_v1.BackendObjectReference{
							Name: "example-svc",
							Port: new(gateway_v1.PortNumber(8000)),
						},
					},
				}},
			}},
		},
	}

	gc := &model.GatewayConfig{
		Routes: []model.GatewayHTTPRouteConfig{{
			HTTPRoute:        httpRouteObject,
			Hostnames:        []gateway_v1.Hostname{"a.localhost.pomerium.io"},
			ValidBackendRefs: noopBackendRefChecker{},
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}},
		ExtensionFilters: map[model.ExtensionFilterKey]model.ExtensionFilter{
			{Kind: "PolicyFilter", Name: "example-policy", Namespace: "test"}: examplePolicyFilter,
		},
	}

	k8sClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// An initial call to SetGatewayConfig() should create a new Pomerium route
	// and policy.
	apiClient.EXPECT().CreatePolicy(ctx, RequestEq(&pomerium.CreatePolicyRequest{
		Policy: &pomerium.Policy{
			OriginatorId: new("ingress-controller"),
			Name:         new("test-example-policy"),
			SourcePpl:    &examplePPL,
		},
	})).Return(createPolicyResponseWithID("example-policy-id"), nil)
	route := &pomerium.Route{
		OriginatorId:         new("ingress-controller"),
		Name:                 new("test-route-a-a-localhost-pomerium-io"),
		From:                 "https://a.localhost.pomerium.io",
		To:                   []string{"http://example-svc.test.svc.cluster.local:8000"},
		LoadBalancingWeights: []uint32{1},
		PreserveHostHeader:   true,
		PolicyIds:            []string{"example-policy-id"},
	}
	apiClient.EXPECT().CreateRoute(ctx, RequestEq(&pomerium.CreateRouteRequest{
		Route: route,
	})).Return(createRouteResponseWithID("new-route-id-1"), nil)

	changed, err := r.SetGatewayConfig(ctx, gc)
	assert.True(t, changed)
	require.NoError(t, err)

	// Modifying the HTTPRoute should update the Pomerium route.
	gc.Routes[0].Spec.Rules[0].BackendRefs[0].BackendObjectReference.Port = new(gateway_v1.PortNumber(1234))

	apiClient.EXPECT().GetPolicy(ctx, RequestEq(&pomerium.GetPolicyRequest{
		Id: "example-policy-id",
	})).Return(connect.NewResponse(&pomerium.GetPolicyResponse{
		Policy: &pomerium.Policy{
			Id:           new("example-policy-id"),
			OriginatorId: new("ingress-controller"),
			Name:         new("test-example-policy"),
			SourcePpl:    &examplePPL,
		},
	}), nil)
	apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
		Id: "new-route-id-1",
	})).Return(connect.NewResponse(&pomerium.GetRouteResponse{
		Route: route,
	}), nil).Times(2)
	apiClient.EXPECT().UpdateRoute(ctx, RequestEq(&pomerium.UpdateRouteRequest{
		Route: &pomerium.Route{
			OriginatorId:         new("ingress-controller"),
			Id:                   new("new-route-id-1"),
			Name:                 new("test-route-a-a-localhost-pomerium-io"),
			From:                 "https://a.localhost.pomerium.io",
			To:                   []string{"http://example-svc.test.svc.cluster.local:1234"},
			LoadBalancingWeights: []uint32{1},
			PreserveHostHeader:   true,
			PolicyIds:            []string{"example-policy-id"},
		},
	})).Return(connect.NewResponse(&pomerium.UpdateRouteResponse{}), nil)

	changed, err = r.SetGatewayConfig(ctx, gc)
	assert.True(t, changed)
	require.NoError(t, err)

	// Deleting the PolicyFilter should delete the Pomerium policy.
	// This leaves a dangling filter reference from the route, making the
	// route invalid.
	policyFilterObject.DeletionTimestamp = new(metav1.Now())

	apiClient.EXPECT().UpdateRoute(ctx, RequestEq(&pomerium.UpdateRouteRequest{
		Route: &pomerium.Route{
			OriginatorId: new("ingress-controller"),
			Id:           new("new-route-id-1"),
			Name:         new("test-route-a-a-localhost-pomerium-io"),
			From:         "https://a.localhost.pomerium.io",
			Response: &pomerium.RouteDirectResponse{
				Status: 500,
				Body:   "invalid filter",
			},
			PreserveHostHeader: true,
			// Note: no PolicyIds
		},
	})).Return(connect.NewResponse(&pomerium.UpdateRouteResponse{}), nil)
	apiClient.EXPECT().DeletePolicy(ctx, RequestEq(&pomerium.DeletePolicyRequest{
		Id: "example-policy-id",
	})).Return(connect.NewResponse(&pomerium.DeletePolicyResponse{}), nil)

	changed, err = r.SetGatewayConfig(ctx, gc)
	assert.True(t, changed)
	require.NoError(t, err)

	// Deleting the HTTPRoute should delete the Pomerium route.
	gc.Routes[0].DeletionTimestamp = new(metav1.Now())

	apiClient.EXPECT().DeleteRoute(ctx, RequestEq(&pomerium.DeleteRouteRequest{
		Id: "new-route-id-1",
	})).Return(connect.NewResponse(&pomerium.DeleteRouteResponse{}), nil)

	changed, err = r.SetGatewayConfig(ctx, gc)
	assert.True(t, changed)
	require.NoError(t, err)
}

func TestAPIReconciler_SetGatewayConfig_routeRecreated(t *testing.T) {
	httpRouteObject := &gateway_v1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-a",
			Namespace: "test",
			Annotations: map[string]string{
				"api.pomerium.io/route-id-0": "existing-route-id",
			},
		},
		Spec: gateway_v1.HTTPRouteSpec{
			CommonRouteSpec: gateway_v1.CommonRouteSpec{},
			Hostnames:       []gateway_v1.Hostname{},
			Rules: []gateway_v1.HTTPRouteRule{{
				BackendRefs: []gateway_v1.HTTPBackendRef{{
					BackendRef: gateway_v1.BackendRef{
						BackendObjectReference: gateway_v1.BackendObjectReference{
							Name: "example-svc",
							Port: new(gateway_v1.PortNumber(8000)),
						},
					},
				}},
			}},
		},
	}

	gc := &model.GatewayConfig{
		Routes: []model.GatewayHTTPRouteConfig{{
			HTTPRoute:        httpRouteObject,
			Hostnames:        []gateway_v1.Hostname{"a.localhost.pomerium.io"},
			ValidBackendRefs: noopBackendRefChecker{},
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "example-svc", Namespace: "test"}: {},
			},
		}},
	}

	apiClient, k8sClient, r := setupReconciler(t)
	ctx := t.Context()

	// If the GetRoute() call returns a Not Found error, the route should be
	// recreated using CreateRoute().
	apiClient.EXPECT().GetRoute(ctx, RequestEq(&pomerium.GetRouteRequest{
		Id: "existing-route-id",
	})).Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))
	apiClient.EXPECT().CreateRoute(ctx, gomock.Any()).
		Return(createRouteResponseWithID("recreated-route-id"), nil)

	k8sClient.EXPECT().Patch(ctx, httpRouteObject, gomock.Any()).Return(nil)

	changed, err := r.SetGatewayConfig(ctx, gc)
	assert.True(t, changed)
	require.NoError(t, err)
	assert.Equal(t, "recreated-route-id", httpRouteObject.Annotations["api.pomerium.io/route-id-0"])
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
					URL:      new("https://idp.example.com"),
					Secret:   "test/idp-client-secret",
				},
				PassIdentityHeaders: new(true),
			},
		},
		IdpSecret: &corev1.Secret{
			Data: map[string][]byte{
				"client_id":     []byte("CLIENT_ID"),
				"client_secret": []byte("CLIENT_SECRET"),
			},
		},
	}

	defaultSettings, err := convertProto[*pomerium.Settings](config.NewDefaultOptions().ToProto().GetSettings())
	require.NoError(t, err)

	t.Run("settings changed", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		// APIReconciler should first call GetSettings() to determine if it needs to sync any changes...
		apiClient.EXPECT().GetSettings(ctx, RequestEq(&pomerium.GetSettingsRequest{})).
			Return(&connect.Response[pomerium.GetSettingsResponse]{
				Msg: &pomerium.GetSettingsResponse{
					Settings: &pomerium.Settings{
						Id: new("settings-id-123"),
					},
				},
			}, nil)

		expectedSettings := proto.CloneOf(defaultSettings)
		expectedSettings.Id = new("settings-id-123")
		expectedSettings.AuthenticateServiceUrl = new("https://authenticate.localhost.pomerium.io")
		expectedSettings.AutocertDir = nil
		expectedSettings.IdpClientId = new("CLIENT_ID")
		expectedSettings.IdpClientSecret = new("CLIENT_SECRET")
		expectedSettings.IdpProvider = new("oidc")
		expectedSettings.IdpProviderUrl = new("https://idp.example.com")
		expectedSettings.PassIdentityHeaders = new(true)
		expectedSettings.RuntimeFlags = nil

		// ...and then call UpdateSettings() once it knows there are changes to sync.
		apiClient.EXPECT().UpdateSettings(ctx, RequestEq(&pomerium.UpdateSettingsRequest{
			Settings: expectedSettings,
		})).Return(&connect.Response[pomerium.UpdateSettingsResponse]{
			Msg: &pomerium.UpdateSettingsResponse{},
		}, nil)

		changes, err := r.SetConfig(ctx, cfg)
		assert.True(t, changes)
		assert.NoError(t, err)
	})

	t.Run("settings unchanged", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		existingSettings := proto.CloneOf(defaultSettings)
		proto.Merge(existingSettings, &pomerium.Settings{
			Id: new("settings-id-123"),

			AuthenticateServiceUrl: new("https://authenticate.localhost.pomerium.io"),
			IdpClientId:            new("CLIENT_ID"),
			IdpClientSecret:        new("CLIENT_SECRET"),
			IdpProvider:            new("oidc"),
			IdpProviderUrl:         new("https://idp.example.com"),
			PassIdentityHeaders:    new(true),

			AutoApplyChangesets: new(true), // this setting should be ignored
		})

		// If the settings already match, there should be no UpdateSettings() call.
		apiClient.EXPECT().GetSettings(ctx, connect.NewRequest(&pomerium.GetSettingsRequest{})).
			Return(&connect.Response[pomerium.GetSettingsResponse]{
				Msg: &pomerium.GetSettingsResponse{
					Settings: existingSettings,
				},
			}, nil)

		changes, err := r.SetConfig(ctx, cfg)
		assert.False(t, changes)
		assert.NoError(t, err)
	})
}

func TestAPIReconciler_syncOneSecret(t *testing.T) {
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

	t.Run("create new cert keypair", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		secret := secretTemplate.DeepCopy()

		// If there is no keypair ID annotation present, APIReconciler should
		// create a new keypair.
		apiClient.EXPECT().CreateKeyPair(ctx, RequestEq(&pomerium.CreateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(createKeyPairResponseWithID("new-keypair-id"), nil)

		// APIReconciler should make a Patch() request to record the
		// newly-assigned ID in the keypair ID annotation (verified below).
		k8sClient.EXPECT().Patch(ctx, secret, gomock.Any()).Return(nil)

		changed, err := r.syncOneSecret(ctx, secret)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Equal(t, "new-keypair-id", secret.Annotations[apiKeyPairIDAnnotation])
		assert.Contains(t, secret.Finalizers, apiFinalizer)
	})

	t.Run("create new CA keypair", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		// APIReconciler should also be able to sync a CA certificate, which
		// has a different representation in the Secret data.
		secret := secretTemplate.DeepCopy()
		secret.Data = map[string][]byte{
			"ca.crt": []byte("ca-cert-data"),
		}

		apiClient.EXPECT().CreateKeyPair(ctx, RequestEq(&pomerium.CreateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("ca-cert-data"),
			},
		})).Return(createKeyPairResponseWithID("new-keypair-id"), nil)

		k8sClient.EXPECT().Patch(ctx, secret, gomock.Any()).Return(nil)

		changed, err := r.syncOneSecret(ctx, secret)
		assert.True(t, changed)
		require.NoError(t, err)
		assert.Equal(t, "new-keypair-id", secret.Annotations[apiKeyPairIDAnnotation])
		assert.Contains(t, secret.Finalizers, apiFinalizer)
	})

	t.Run("update existing keypair", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		secret := secretTemplate.DeepCopy()
		secret.Annotations = map[string]string{
			apiKeyPairIDAnnotation: "existing-keypair-id",
		}

		// APIReconciler should first call GetKeyPair() to determine if it needs to sync any changes...
		apiClient.EXPECT().GetKeyPair(ctx, RequestEq(&pomerium.GetKeyPairRequest{
			Id: "existing-keypair-id",
		})).Return(&connect.Response[pomerium.GetKeyPairResponse]{
			Msg: &pomerium.GetKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					OriginatorId: new("ingress-controller"),
					Id:           new("existing-keypair-id"),
					Name:         new("test-secret-1"),
					Certificate:  []byte("different-cert-data"),
					Key:          []byte("different-key-data"),
				},
			},
		}, nil)

		// ...and then UpdateKeyPair() to sync changes.
		apiClient.EXPECT().UpdateKeyPair(ctx, RequestEq(&pomerium.UpdateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: new("ingress-controller"),
				Id:           new("existing-keypair-id"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(&connect.Response[pomerium.UpdateKeyPairResponse]{
			Msg: &pomerium.UpdateKeyPairResponse{},
		}, nil)

		k8sClient.EXPECT().Patch(ctx, secret, gomock.Any()).Return(nil)

		changed, err := r.syncOneSecret(ctx, secret)
		assert.True(t, changed)
		assert.NoError(t, err)
	})

	t.Run("update error", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		secret := secretTemplate.DeepCopy()
		secret.Annotations = map[string]string{
			apiKeyPairIDAnnotation: "existing-keypair-id",
		}

		// APIReconciler should first call GetKeyPair() to determine if it needs to sync any changes...
		apiClient.EXPECT().GetKeyPair(ctx, RequestEq(&pomerium.GetKeyPairRequest{
			Id: "existing-keypair-id",
		})).Return(&connect.Response[pomerium.GetKeyPairResponse]{
			Msg: &pomerium.GetKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					OriginatorId: new("ingress-controller"),
					Id:           new("existing-keypair-id"),
					Name:         new("test-secret-1"),
					Certificate:  []byte("different-cert-data"),
					Key:          []byte("different-key-data"),
				},
			},
		}, nil)

		// ...and then UpdateKeyPair() to sync changes. If this returns an error, it
		// should be surfaced.
		apiClient.EXPECT().UpdateKeyPair(ctx, RequestEq(&pomerium.UpdateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: new("ingress-controller"),
				Id:           new("existing-keypair-id"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(nil, connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded))

		changed, err := r.syncOneSecret(ctx, secret)
		assert.False(t, changed)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("existing keypair unchanged", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		secret := secretTemplate.DeepCopy()
		secret.Annotations = map[string]string{
			apiKeyPairIDAnnotation: "existing-keypair-id",
		}

		// If the keypair already matches, there should be no UpdateKeyPair() call.
		apiClient.EXPECT().GetKeyPair(ctx, RequestEq(&pomerium.GetKeyPairRequest{
			Id: "existing-keypair-id",
		})).Return(&connect.Response[pomerium.GetKeyPairResponse]{
			Msg: &pomerium.GetKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					OriginatorId: new("ingress-controller"),
					Id:           new("existing-keypair-id"),
					Name:         new("test-secret-1"),
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

		changed, err := r.syncOneSecret(ctx, secret)
		assert.False(t, changed)
		assert.NoError(t, err)
	})

	t.Run("already exists", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		secret := secretTemplate.DeepCopy()

		// If the keypair already exists, but we failed to save its ID, we
		// should still be able to look up the keypair by name.
		apiClient.EXPECT().CreateKeyPair(ctx, RequestEq(&pomerium.CreateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("already exists")))

		apiClient.EXPECT().ListKeyPairs(ctx, RequestEq(&pomerium.ListKeyPairsRequest{
			Filter: filterByName(t, "test-secret-1"),
		})).Return(connect.NewResponse(&pomerium.ListKeyPairsResponse{
			KeyPairs: []*pomerium.KeyPair{{
				OriginatorId: new("ingress-controller"),
				Id:           new("missing-keypair-id"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),

				// these fields should be ignored
				CertificateInfo: []*pomerium.CertificateInfo{{
					Version: 1234,
					Serial:  "ABCD",
				}},
				Status: pomerium.KeyPairStatus_KEY_PAIR_STATUS_READY,
				Origin: pomerium.KeyPairOrigin_KEY_PAIR_ORIGIN_USER,
			}},
		}), nil)

		k8sClient.EXPECT().Patch(ctx, secret, gomock.Any()).Return(nil)

		changed, err := r.syncOneSecret(ctx, secret)
		assert.True(t, changed)
		assert.NoError(t, err)
		assert.Equal(t, "missing-keypair-id", secret.Annotations["api.pomerium.io/keypair-id"])
		assert.Contains(t, secret.Finalizers, apiFinalizer)
	})

	t.Run("existing keypair not found", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		secret := secretTemplate.DeepCopy()
		secret.Annotations = map[string]string{
			apiKeyPairIDAnnotation: "existing-keypair-id",
		}

		// If there is an existing keypair ID annotation present, but it cannot
		// be retrieved, APIReconciler should create it as a new keypair.
		apiClient.EXPECT().GetKeyPair(ctx, RequestEq(&pomerium.GetKeyPairRequest{
			Id: "existing-keypair-id",
		})).Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))

		apiClient.EXPECT().CreateKeyPair(ctx, RequestEq(&pomerium.CreateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				Id:           new("existing-keypair-id"),
				OriginatorId: new("ingress-controller"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(&connect.Response[pomerium.CreateKeyPairResponse]{
			Msg: &pomerium.CreateKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					Id: new("existing-keypair-id"),
					// rest of the data omitted (not currently read)
				},
			},
		}, nil)

		k8sClient.EXPECT().Patch(ctx, secret, gomock.Any()).Return(nil)

		changed, err := r.syncOneSecret(ctx, secret)
		assert.True(t, changed)
		assert.NoError(t, err)
	})

	t.Run("patch error", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()

		secret := secretTemplate.DeepCopy()

		apiClient.EXPECT().CreateKeyPair(ctx, RequestEq(&pomerium.CreateKeyPairRequest{
			KeyPair: &pomerium.KeyPair{
				OriginatorId: new("ingress-controller"),
				Name:         new("test-secret-1"),
				Certificate:  []byte("cert-data"),
				Key:          []byte("key-data"),
			},
		})).Return(&connect.Response[pomerium.CreateKeyPairResponse]{
			Msg: &pomerium.CreateKeyPairResponse{
				KeyPair: &pomerium.KeyPair{
					Id: new("new-keypair-id"),
					// rest of the data omitted (not currently read)
				},
			},
		}, nil)

		// If the metadata patch operation fails, this error should be surfaced.
		patchErr := fmt.Errorf("failed to patch")
		k8sClient.EXPECT().Patch(ctx, secret, gomock.Any()).Return(patchErr)

		changed, err := r.syncOneSecret(ctx, secret)
		assert.True(t, changed)
		require.ErrorIs(t, err, patchErr)
	})
}

func TestAPIReconciler_deleteKeyPairs(t *testing.T) {
	t.Run("secret missing", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()
		n := types.NamespacedName{Namespace: "test", Name: "my-secret"}

		// If a Secret was already deleted, we should look for a corresponding
		// keypair by name.
		k8sClient.EXPECT().Get(ctx, n, gomock.AssignableToTypeOf((*corev1.Secret)(nil))).
			Return(apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, n.Name))
		apiClient.EXPECT().ListKeyPairs(ctx, connect.NewRequest(&pomerium.ListKeyPairsRequest{
			Filter: filterByName(t, "test-my-secret"),
		})).Return(connect.NewResponse(&pomerium.ListKeyPairsResponse{
			KeyPairs: []*pomerium.KeyPair{{
				Id: new("my-keypair-id"),
			}},
		}), nil)
		apiClient.EXPECT().DeleteKeyPair(ctx, connect.NewRequest(&pomerium.DeleteKeyPairRequest{
			Id: "my-keypair-id",
		}))

		_, err := r.deleteKeyPairs(ctx, n)
		assert.NoError(t, err)
	})

	t.Run("secret and keypair missing", func(t *testing.T) {
		apiClient, k8sClient, r := setupReconciler(t)
		ctx := t.Context()
		n := types.NamespacedName{Namespace: "test", Name: "my-secret"}

		// If a Secret was already deleted, and no matching keypair exists,
		// there should be no error returned.
		k8sClient.EXPECT().Get(ctx, n, gomock.AssignableToTypeOf((*corev1.Secret)(nil))).
			Return(apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, n.Name))
		apiClient.EXPECT().ListKeyPairs(ctx, connect.NewRequest(&pomerium.ListKeyPairsRequest{
			Filter: filterByName(t, "test-my-secret"),
		})).Return(connect.NewResponse(&pomerium.ListKeyPairsResponse{KeyPairs: nil}), nil)

		_, err := r.deleteKeyPairs(ctx, n)
		assert.NoError(t, err)
	})
}

func TestAPIReconciler_upsertPolicy(t *testing.T) {
	policy := &pomerium.Policy{
		Id: new("existing-policy-id"),
	}

	t.Run("not found error", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		// When trying to update a policy, if the policy does not currently
		// exist, it should be recreated using a CreatePolicy() request.
		apiClient.EXPECT().GetPolicy(ctx, RequestEq(&pomerium.GetPolicyRequest{
			Id: "existing-policy-id",
		})).Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))

		apiClient.EXPECT().CreatePolicy(ctx, RequestEq(&pomerium.CreatePolicyRequest{
			Policy: policy,
		})).Return(createPolicyResponseWithID("existing-policy-id"), nil)

		changed, err := r.upsertPolicy(ctx, policy)
		assert.True(t, changed)
		assert.NoError(t, err)
	})

	t.Run("other error", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		apiError := connect.NewError(connect.CodeUnavailable, fmt.Errorf("unavailable"))

		apiClient.EXPECT().GetPolicy(ctx, RequestEq(&pomerium.GetPolicyRequest{
			Id: "existing-policy-id",
		})).Return(nil, apiError)

		changed, err := r.upsertPolicy(ctx, policy)
		assert.False(t, changed)
		assert.Equal(t, apiError, err)
	})

	t.Run("no changes needed", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		// When determining if a policy needs to be updated, certain fields
		// should be ignored.
		apiClient.EXPECT().GetPolicy(ctx, RequestEq(&pomerium.GetPolicyRequest{
			Id: "existing-policy-id",
		})).Return(connect.NewResponse(&pomerium.GetPolicyResponse{
			Policy: &pomerium.Policy{
				Id: new("existing-policy-id"),

				NamespaceId: new("some-namespace-id"),
				CreatedAt:   timestamppb.Now(),
				ModifiedAt:  timestamppb.Now(),
				AssignedRoutes: []*pomerium.EntityInfo{{
					Id:   new("some-route-id"),
					Name: new("some-route-name"),
				}},
				Enforced: new(false),
			},
		}), nil)

		// No UpdatePolicy() call expected.

		changed, err := r.upsertPolicy(ctx, policy)
		assert.False(t, changed)
		assert.NoError(t, err)
	})

	t.Run("already exists", func(t *testing.T) {
		policy := &pomerium.Policy{
			OriginatorId: new("originator-id"),
			Name:         new("policy-name"),
		}

		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		// If we try to create a policy but it already exists, we should attempt
		// to look up the existing policy by name.
		apiClient.EXPECT().CreatePolicy(ctx, RequestEq(&pomerium.CreatePolicyRequest{
			Policy: &pomerium.Policy{
				OriginatorId: new("originator-id"),
				Name:         new("policy-name"),
			},
		})).Return(nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("already exists")))

		apiClient.EXPECT().ListPolicies(ctx, RequestEq(&pomerium.ListPoliciesRequest{
			Filter: filterByName(t, "policy-name"),
		})).Return(connect.NewResponse(&pomerium.ListPoliciesResponse{
			Policies: []*pomerium.Policy{{
				OriginatorId: new("originator-id"),
				Id:           new("missing-policy-id"),
				Name:         new("policy-name"),
			}},
		}), nil)

		changed, err := r.upsertPolicy(ctx, policy)
		assert.True(t, changed)
		assert.NoError(t, err)
		assert.Equal(t, "missing-policy-id", policy.GetId())
	})
}

func TestAPIReconciler_deletePolicy(t *testing.T) {
	t.Run("not found error", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"api.pomerium.io/policy-id": "existing-policy-id",
				},
			},
		}

		// If a DeletePolicy() call results in a Not Found error, APIReconciler
		// should not propagate this error (there was no need to delete the policy
		// in the first place).
		apiClient.EXPECT().DeletePolicy(ctx, RequestEq(&pomerium.DeletePolicyRequest{
			Id: "existing-policy-id",
		})).Return(nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("not found")))

		deleted, err := r.deletePolicy(ctx, ingress)
		assert.True(t, deleted)
		assert.NoError(t, err)
		assert.NotContains(t, ingress.Annotations, "api.pomerium.io/policy-id")
	})

	t.Run("other error", func(t *testing.T) {
		apiClient, _, r := setupReconciler(t)
		ctx := t.Context()

		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"api.pomerium.io/policy-id": "existing-policy-id",
				},
			},
		}

		apiError := connect.NewError(connect.CodeUnavailable, fmt.Errorf("unavailable"))

		apiClient.EXPECT().DeletePolicy(ctx, RequestEq(&pomerium.DeletePolicyRequest{
			Id: "existing-policy-id",
		})).Return(nil, apiError)

		deleted, err := r.deletePolicy(ctx, ingress)
		assert.False(t, deleted)
		assert.Equal(t, apiError, err)
		assert.Equal(t, "existing-policy-id", ingress.Annotations["api.pomerium.io/policy-id"])
	})
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

func createPolicyResponseWithID(id string) *connect.Response[pomerium.CreatePolicyResponse] {
	return &connect.Response[pomerium.CreatePolicyResponse]{
		Msg: &pomerium.CreatePolicyResponse{
			Policy: &pomerium.Policy{
				Id: &id,
				// the rest of the policy data is not currently read
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

// Implementation of BackendRefChecker that simply allows all BackendRefs.
type noopBackendRefChecker struct{}

func (noopBackendRefChecker) Valid(_ client.Object, _ *gateway_v1.BackendRef) bool {
	return true
}

func filterByName(t *testing.T, name string) *structpb.Struct {
	f, err := structpb.NewStruct(map[string]any{
		"originatorId": "ingress-controller",
		"name":         name,
	})
	require.NoError(t, err)
	return f
}

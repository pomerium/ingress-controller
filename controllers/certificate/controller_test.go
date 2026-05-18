package certificate

import (
	"context"
	"testing"

	certmanager_api "github.com/cert-manager/cert-manager/pkg/api"
	certmanager_v1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanager_meta_v1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/pomerium/ingress-controller/internal/certificate"
)

func TestReconcileCertificates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	namespace := "default"
	issuer := certmanager_meta_v1.IssuerReference{Kind: "ClusterIssuer", Name: "letsencrypt"}

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, certmanager_api.AddToScheme(scheme))

	newController := func(t *testing.T, initObjs ...client.Object) *certificateController {
		t.Helper()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).Build()
		c := &certificateController{kubernetesClient: cl}
		c.dataBrokerCollector = newDataBrokerCollector(c)
		return c
	}

	initDataBroker := func(t *testing.T, c *certificateController, routesByKey map[recordKey][]string) {
		t.Helper()
		matcher := certificate.NewMatcher[recordKey]()
		for key, names := range routesByKey {
			matcher.Update(key, nil, names)
		}
		c.dataBrokerCollector.matcher = matcher
		started := make(chan struct{})
		c.dataBrokerCollector.operation.Start(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		})
		<-started
		t.Cleanup(c.dataBrokerCollector.operation.StopNow)
	}

	makeCert := func(name string, ref certmanager_meta_v1.IssuerReference, dnsNames ...string) *certmanager_v1.Certificate {
		return &certmanager_v1.Certificate{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: certmanager_v1.CertificateSpec{
				IssuerRef:  ref,
				DNSNames:   dnsNames,
				SecretName: name + "-secret",
			},
		}
	}

	listCerts := func(t *testing.T, c *certificateController) []certmanager_v1.Certificate {
		t.Helper()
		var cl certmanager_v1.CertificateList
		require.NoError(t, c.kubernetesClient.List(ctx, &cl, client.InNamespace(namespace)))
		return cl.Items
	}

	t.Run("no issuer skips provisioning", func(t *testing.T) {
		c := newController(t)
		err := c.reconcileCertificates(ctx, namespace, certmanager_meta_v1.IssuerReference{}, nil)
		assert.NoError(t, err)
		assert.Empty(t, listCerts(t, c))
	})

	t.Run("no issuer deletes mismatched certs but does not provision", func(t *testing.T) {
		other := certmanager_meta_v1.IssuerReference{Kind: "ClusterIssuer", Name: "other"}
		cert := makeCert("a", other, "a.example.com")
		secret := &core_v1.Secret{ObjectMeta: meta_v1.ObjectMeta{Namespace: namespace, Name: "a-secret"}}
		c := newController(t, cert, secret)

		err := c.reconcileCertificates(ctx, namespace, certmanager_meta_v1.IssuerReference{}, []certmanager_v1.Certificate{*cert})
		require.NoError(t, err)

		assert.Empty(t, listCerts(t, c), "mismatched cert should be deleted")
		err = c.kubernetesClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "a-secret"}, &core_v1.Secret{})
		assert.True(t, apierrors.IsNotFound(err), "secret should be deleted")
	})

	t.Run("keeps certificate that covers a missing name", func(t *testing.T) {
		cert := makeCert("a", issuer, "a.example.com")
		c := newController(t, cert)
		initDataBroker(t, c, map[recordKey][]string{
			{"r", "1"}: {"a.example.com"},
		})

		err := c.reconcileCertificates(ctx, namespace, issuer, []certmanager_v1.Certificate{*cert})
		require.NoError(t, err)

		remaining := listCerts(t, c)
		require.Len(t, remaining, 1)
		assert.Equal(t, "a", remaining[0].Name)
	})

	t.Run("deletes certificate that no longer covers any route", func(t *testing.T) {
		certA := makeCert("a", issuer, "a.example.com")
		certB := makeCert("b", issuer, "b.example.com")
		c := newController(t, certA, certB)
		initDataBroker(t, c, map[recordKey][]string{
			{"r", "1"}: {"b.example.com"},
		})

		err := c.reconcileCertificates(ctx, namespace, issuer, []certmanager_v1.Certificate{*certA, *certB})
		require.NoError(t, err)

		remaining := listCerts(t, c)
		require.Len(t, remaining, 1, "only the cert covering a missing route should remain")
		assert.Equal(t, "b", remaining[0].Name)
	})

	t.Run("creates certificate for missing name", func(t *testing.T) {
		c := newController(t)
		initDataBroker(t, c, map[recordKey][]string{
			{"r", "1"}: {"new.example.com"},
		})

		err := c.reconcileCertificates(ctx, namespace, issuer, nil)
		require.NoError(t, err)

		created := listCerts(t, c)
		require.Len(t, created, 1)
		got := created[0]
		assert.Equal(t, namespace, got.Namespace)
		assert.Equal(t, []string{"new.example.com"}, got.Spec.DNSNames)
		assert.Equal(t, issuer, got.Spec.IssuerRef)
		assert.Equal(t, managedByLabelValue, got.Labels[managedByLabelName])
		assert.NotEmpty(t, got.Spec.SecretName)
		require.NotNil(t, got.Spec.SecretTemplate)
		assert.Equal(t, managedByLabelValue, got.Spec.SecretTemplate.Labels[managedByLabelName])
	})

	t.Run("creates certificate when issuer changes", func(t *testing.T) {
		other := certmanager_meta_v1.IssuerReference{Kind: "ClusterIssuer", Name: "other"}
		cert := makeCert("a", other, "a.example.com")
		secret := &core_v1.Secret{ObjectMeta: meta_v1.ObjectMeta{Namespace: namespace, Name: "a-secret"}}
		c := newController(t, cert, secret)
		initDataBroker(t, c, map[recordKey][]string{
			{"r", "1"}: {"a.example.com"},
		})

		err := c.reconcileCertificates(ctx, namespace, issuer, []certmanager_v1.Certificate{*cert})
		require.NoError(t, err)

		created := listCerts(t, c)
		if assert.Len(t, created, 1) {
			assert.Equal(t, issuer, created[0].Spec.IssuerRef)
		}
	})
}

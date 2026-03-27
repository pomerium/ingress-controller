package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

func TestKeyForObject(t *testing.T) {
	i := &networkingv1.Ingress{
		TypeMeta: v1.TypeMeta{
			Kind: "Ingress",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "ingress-1",
			Namespace: "test",
		},
	}
	assert.Equal(t, Key{
		Kind:           "Ingress",
		NamespacedName: types.NamespacedName{Name: "ingress-1", Namespace: "test"},
	}, KeyForObject(i))
}

func TestTLSSecretsMap_UpdateIngress(t *testing.T) {
	ic1 := &IngressConfig{
		Ingress: &networkingv1.Ingress{
			TypeMeta: v1.TypeMeta{
				Kind: "Ingress",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "ingress-1",
				Namespace: "test",
			},
		},
		AnnotationPrefix: "a",
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "secret-a", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
			{Name: "secret-b", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
			{Name: "secret-c", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
			{Name: "secret-d", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
			{Name: "not-a-tls-secret", Namespace: "test"}: {
				// this one should be ignored because it's not a TLS secret
				Type: corev1.SecretTypeOpaque,
			},
		},
	}

	ic2 := &IngressConfig{
		Ingress: &networkingv1.Ingress{
			TypeMeta: v1.TypeMeta{
				Kind: "Ingress",
			}, ObjectMeta: v1.ObjectMeta{
				Name:      "ingress-2",
				Namespace: "test",
			},
		},
		AnnotationPrefix: "a",
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "secret-d", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
		},
	}

	m := NewTLSSecretsMap()
	assert.Empty(t, m.UpdateIngress(ic1))
	assert.Empty(t, m.UpdateIngress(ic2))

	// If there are no changes, there should be no newly-unreferenced secrets.
	assert.Empty(t, m.UpdateIngress(ic1))
	assert.Empty(t, m.UpdateIngress(ic2))

	// Remove the one reference to secret A.
	delete(ic1.Secrets, types.NamespacedName{Name: "secret-a", Namespace: "test"})
	assert.Equal(t, []types.NamespacedName{{Name: "secret-a", Namespace: "test"}}, m.UpdateIngress(ic1))

	// Remove the references to secrets B and C. (Secret D is still referenced by ingress 2.)
	clear(ic1.Secrets)
	assert.ElementsMatch(t, []types.NamespacedName{
		{Name: "secret-b", Namespace: "test"},
		{Name: "secret-c", Namespace: "test"},
	}, m.UpdateIngress(ic1))

	// Remove the last reference to secret D.
	delete(ic2.Secrets, types.NamespacedName{Name: "secret-d", Namespace: "test"})
	assert.Equal(t, []types.NamespacedName{{Name: "secret-d", Namespace: "test"}}, m.UpdateIngress(ic2))
}

func TestTLSSecretsMap_UpdateConfigAndUpdateIngress(t *testing.T) {
	ic1 := &IngressConfig{
		Ingress: &networkingv1.Ingress{
			TypeMeta: v1.TypeMeta{
				Kind: "Ingress",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "ingress-1",
				Namespace: "test",
			},
		},
		AnnotationPrefix: "a",
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "secret-a", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
			{Name: "secret-b", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
			{Name: "secret-c", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
		},
	}

	cfg := &Config{
		Pomerium: icsv1.Pomerium{
			TypeMeta: v1.TypeMeta{
				Kind: "Pomerium",
			}, ObjectMeta: v1.ObjectMeta{
				Name: "global",
			},
		},
		Certs: map[types.NamespacedName]*corev1.Secret{
			{Name: "secret-b", Namespace: "test"}: {},
		},
		CASecrets: []*corev1.Secret{
			{
				ObjectMeta: v1.ObjectMeta{
					Name: "secret-c", Namespace: "test",
				},
			},
		},
	}

	m := NewTLSSecretsMap()
	assert.Empty(t, m.UpdateIngress(ic1))
	assert.Empty(t, m.UpdateConfig(cfg))

	// Secrets B and C are referenced by both entities. Removing a single reference
	// should not result in either becoming unreferenced.
	delete(ic1.Secrets, types.NamespacedName{Name: "secret-b", Namespace: "test"})
	assert.Empty(t, m.UpdateIngress(ic1))
	cfg.CASecrets = nil
	assert.Empty(t, m.UpdateConfig(cfg))

	// Now remove the other reference.
	delete(ic1.Secrets, types.NamespacedName{Name: "secret-c", Namespace: "test"})
	assert.Equal(t, []types.NamespacedName{{Name: "secret-c", Namespace: "test"}}, m.UpdateIngress(ic1))
	delete(cfg.Certs, types.NamespacedName{Name: "secret-b", Namespace: "test"})
	assert.Equal(t, []types.NamespacedName{{Name: "secret-b", Namespace: "test"}}, m.UpdateConfig(cfg))
}

func TestTLSSecretsMap_RemoveEntity(t *testing.T) {
	foo1 := Key{Kind: "Foo", NamespacedName: types.NamespacedName{Name: "foo-1", Namespace: "test"}}
	foo2 := Key{Kind: "Foo", NamespacedName: types.NamespacedName{Name: "foo-2", Namespace: "test"}}

	m := NewTLSSecretsMap()
	m.Add(foo1, types.NamespacedName{Name: "secret-a", Namespace: "test"})
	m.Add(foo1, types.NamespacedName{Name: "secret-b", Namespace: "test"})
	m.Add(foo2, types.NamespacedName{Name: "secret-a", Namespace: "test"})

	assert.Equal(t, []types.NamespacedName{{Name: "secret-b", Namespace: "test"}}, m.RemoveEntity(foo1))
	assert.Equal(t, []types.NamespacedName{{Name: "secret-a", Namespace: "test"}}, m.RemoveEntity(foo2))
}

func TestTLSSecretsMap_Reset(t *testing.T) {
	ic1 := &IngressConfig{
		Ingress: &networkingv1.Ingress{
			ObjectMeta: v1.ObjectMeta{
				Name:      "ingress-1",
				Namespace: "test",
			},
		},
		AnnotationPrefix: "a",
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "secret-a", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
			},
		},
	}

	m := NewTLSSecretsMap()
	assert.Empty(t, m.UpdateIngress(ic1))

	// If we reset the map before a modification, UpdateIngress should return
	// an empty slice of unreferenced secret names.
	m.Reset()
	delete(ic1.Secrets, types.NamespacedName{Name: "secret-a", Namespace: "test"})
	assert.Empty(t, m.UpdateIngress(ic1))
}

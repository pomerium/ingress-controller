package util

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/pomerium/pkg/cryptutil"
)

// NewBootstrapSecrets generate secrets for pomerium bootstrap
func NewBootstrapSecrets(name types.NamespacedName) (*corev1.Secret, error) {
	key, err := cryptutil.NewSigningKey()
	if err != nil {
		return nil, fmt.Errorf("gen key: %w", err)
	}
	signingKey, err := cryptutil.EncodePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("pem: %w", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
		Data: map[string][]byte{
			"shared_secret": cryptutil.NewKey(),
			"cookie_secret": cryptutil.NewKey(),
			"signing_key":   signingKey,
		},
		Type: corev1.SecretTypeOpaque,
	}, nil
}

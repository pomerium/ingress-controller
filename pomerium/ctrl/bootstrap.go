// Package ctrl converts Settings CRD into a bootstrap config
package ctrl

import (
	"encoding/base64"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/pomerium/config"

	"github.com/pomerium/ingress-controller/model"
)

// Make prepares a minimal bootstrap configuration for Pomerium
func Make(src *model.Config) (*config.Options, error) {
	opts := config.NewDefaultOptions()

	for _, apply := range []struct {
		name string
		fn   func(*config.Options, *model.Config) error
	}{
		{"secrets", applySecrets},
		{"metrics", applyDebugMetrics},
	} {
		if err := apply.fn(opts, src); err != nil {
			return nil, fmt.Errorf("%s: %w", apply.name, err)
		}
	}

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return opts, nil
}

func applyDebugMetrics(dst *config.Options, src *model.Config) error {
	dst.MetricsAddr = "localhost:8080"
	return nil
}

func applySecrets(dst *config.Options, src *model.Config) error {
	if src.Secrets == nil {
		return fmt.Errorf("secrets missing, this is a bug")
	}

	name := types.NamespacedName{Name: src.Secrets.Name, Namespace: src.Secrets.Namespace}

	for _, secret := range []struct {
		key string
		len int
		sp  *string
	}{
		{"shared_secret", 32, &dst.SharedKey},
		{"cookie_secret", 32, &dst.CookieSecret},
		{"signing_key", -1, &dst.SigningKey},
	} {
		data, ok := src.Secrets.Data[secret.key]
		if !ok && secret.len > 0 {
			return fmt.Errorf("secret %s is missing a key %s", name, secret.key)
		}
		if secret.len > 0 && len(data) != secret.len {
			return fmt.Errorf("secret %s, key %s should be %d bytes, got %d", name, secret.key, secret.len, len(data))
		}
		txt := base64.StdEncoding.EncodeToString(data)
		*secret.sp = txt
	}

	return nil
}

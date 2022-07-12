//Package settings implements controller for Settings CRD
package settings

import (
	context "context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
)

// FetchConfig returns
func FetchConfig(ctx context.Context, client client.Client, name types.NamespacedName) (*model.Config, error) {
	var cfg model.Config
	if err := client.Get(ctx, name, &cfg.Pomerium); err != nil {
		return nil, fmt.Errorf("get %s: %w", name, err)
	}

	if err := fetchConfigSecrets(ctx, client, &cfg); err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}

	if err := fetchConfigCerts(ctx, client, &cfg); err != nil {
		return nil, fmt.Errorf("certs: %w", err)
	}

	return &cfg, nil
}

func fetchConfigCerts(ctx context.Context, client client.Client, cfg *model.Config) error {
	if cfg.Certs == nil {
		cfg.Certs = make(map[types.NamespacedName]*corev1.Secret)
	}

	for _, src := range cfg.Spec.Certificates {
		name, err := util.ParseNamespacedName(src)
		if err != nil {
			return fmt.Errorf("parse %s: %w", src, err)
		}
		var secret corev1.Secret
		if err := client.Get(ctx, *name, &secret); err != nil {
			return fmt.Errorf("get %s: %w", name, err)
		}
		cfg.Certs[*name] = &secret
	}

	return nil
}

func fetchConfigSecrets(ctx context.Context, client client.Client, cfg *model.Config) error {
	get := func(src string) func() (*corev1.Secret, error) {
		return func() (*corev1.Secret, error) {
			name, err := util.ParseNamespacedName(src)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", src, err)
			}
			var secret corev1.Secret
			if err := client.Get(ctx, *name, &secret); err != nil {
				return nil, fmt.Errorf("get %s: %w", name, err)
			}
			return &secret, nil
		}
	}
	optional := func(src *string) func() (*corev1.Secret, error) {
		if src == nil {
			return func() (*corev1.Secret, error) { return nil, nil }
		}
		return get(*src)
	}
	required := func(src *string) func() (*corev1.Secret, error) {
		if src == nil {
			return func() (*corev1.Secret, error) { return nil, fmt.Errorf("required") }
		}
		return get(*src)
	}
	apply := func(name string, getFn func() (*corev1.Secret, error), dst **corev1.Secret) func() error {
		return func() error {
			secret, err := getFn()
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if secret != nil {
				*dst = secret
			}
			return nil
		}
	}
	applyAll := func(funcs ...func() error) error {
		for _, fn := range funcs {
			if err := fn(); err != nil {
				return err
			}
		}
		return nil
	}

	s := cfg.Spec
	return applyAll(
		apply("bootstrap secret", required(&s.Secrets), &cfg.Secrets),
		apply("secret", required(&s.IdentityProvider.Secret), &cfg.IdpSecret),
		apply("request params", optional(s.IdentityProvider.RequestParamsSecret), &cfg.RequestParams),
		apply("service account", optional(s.IdentityProvider.ServiceAccountFromSecret), &cfg.IdpServiceAccount),
		func() error {
			if s.Storage == nil {
				return nil
			}

			if r := s.Storage.Redis; r != nil {
				if err := applyAll(
					apply("connection", required(&r.Secret), &cfg.StorageSecrets.Secret),
					apply("tls", optional(r.TLSSecret), &cfg.StorageSecrets.TLS),
					apply("ca", optional(r.CASecret), &cfg.StorageSecrets.CA),
				); err != nil {
					return fmt.Errorf("redis: %w", err)
				}
			} else if p := s.Storage.Postgres; p != nil {
				if err := applyAll(
					apply("connection", required(&p.Secret), &cfg.StorageSecrets.Secret),
					apply("tls", optional(p.TLSSecret), &cfg.StorageSecrets.TLS),
					apply("ca", optional(p.CASecret), &cfg.StorageSecrets.CA),
				); err != nil {
					return fmt.Errorf("postgresql: %w", err)
				}
			} else {
				return fmt.Errorf("if storage is specified, either redis or postgres storage should be provided")
			}

			return cfg.StorageSecrets.Validate()
		},
	)
}

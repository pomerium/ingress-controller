// Package settings implements controller for Settings CRD
package settings

import (
	context "context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
		return &cfg, fmt.Errorf("secrets: %w", err)
	}

	if err := fetchConfigCerts(ctx, client, &cfg); err != nil {
		return &cfg, fmt.Errorf("certs: %w", err)
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
			log := log.FromContext(ctx)
			log.Info("certificate secret not found, skipping", "secret", name, "error", err)
			continue
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
		// bootstrap secrets
		apply("bootstrap secret", required(&s.Secrets), &cfg.Secrets),
		// ca secrets
		func() error {
			for _, caSecret := range s.CASecrets {
				secret, err := get(caSecret)()
				if err != nil {
					return fmt.Errorf("ca: %w", err)
				}
				cfg.CASecrets = append(cfg.CASecrets, secret)
			}
			return nil
		},
		// identity provider secrets
		func() error {
			if s.IdentityProvider == nil {
				return nil
			}
			return applyAll(
				apply("secret", required(&s.IdentityProvider.Secret), &cfg.IdpSecret),
				apply("request params", optional(s.IdentityProvider.RequestParamsSecret), &cfg.RequestParams),
				apply("service account", optional(s.IdentityProvider.ServiceAccountFromSecret), &cfg.IdpServiceAccount),
			)
		},
		// ssh secrets
		func() error {
			if s.SSH == nil {
				return nil
			}

			if sshHostKeySecrets := s.SSH.HostKeySecrets; sshHostKeySecrets != nil {
				for _, sshHostKeySecret := range *sshHostKeySecrets {
					secret, err := get(sshHostKeySecret)()
					if err != nil {
						return fmt.Errorf("error retrieving ssh host key secret (%s): %w", sshHostKeySecret, err)
					}
					cfg.SSHSecrets.HostKeys = append(cfg.SSHSecrets.HostKeys, secret)
				}
			}

			if sshUserCAKeySecret := s.SSH.UserCAKeySecret; sshUserCAKeySecret != nil {
				secret, err := get(*sshUserCAKeySecret)()
				if err != nil {
					return fmt.Errorf("error retrieving ssh user ca key secret (%s): %w", *sshUserCAKeySecret, err)
				}
				cfg.SSHSecrets.UserCAKey = secret
			}

			return cfg.SSHSecrets.Validate()
		},
		// storage secrets
		func() error {
			if s.Storage == nil {
				return nil
			}

			if f := s.Storage.File; f != nil {
				return nil
			}

			if p := s.Storage.Postgres; p != nil {
				if err := applyAll(
					apply("connection", required(&p.Secret), &cfg.StorageSecrets.Secret),
					apply("tls", optional(p.TLSSecret), &cfg.StorageSecrets.TLS),
					apply("ca", optional(p.CASecret), &cfg.StorageSecrets.CA),
				); err != nil {
					return fmt.Errorf("postgres: %w", err)
				}
			} else {
				return fmt.Errorf("if storage is specified, file or postgres storage should be provided")
			}

			return cfg.StorageSecrets.Validate()
		},
	)
}

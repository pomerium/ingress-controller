package pomerium

import (
	"cmp"
	"fmt"
	"maps"
	"net/url"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func addCerts(cfg *pb.Config, secrets map[types.NamespacedName]*corev1.Secret) {
	if cfg.Settings == nil {
		cfg.Settings = new(pb.Settings)
	}

	keys := slices.Collect(maps.Keys(secrets))
	slices.SortStableFunc(keys, func(a, b types.NamespacedName) int {
		if cmp := cmp.Compare(a.Namespace, b.Namespace); cmp != 0 {
			return cmp
		}
		return cmp.Compare(a.Name, b.Name)
	})
	for _, key := range keys {
		secret := secrets[key]
		if secret.Type != corev1.SecretTypeTLS {
			continue
		}
		addTLSCert(cfg.Settings, secret)
	}
}

func addTLSCert(s *pb.Settings, secret *corev1.Secret) {
	s.Certificates = append(s.Certificates, &pb.Settings_Certificate{
		CertBytes: secret.Data[corev1.TLSCertKey],
		KeyBytes:  secret.Data[corev1.TLSPrivateKeyKey],
	})
}

func removeUnusedCerts(cfg *pb.Config) error {
	if cfg.Settings == nil {
		return nil
	}

	dm, err := toDomainMap(cfg.Settings.Certificates)
	if err != nil {
		return err
	}

	domains, err := getAllDomains(cfg)
	if err != nil {
		return err
	}

	for domain := range domains {
		dm.markInUse(domain)
	}

	cfg.Settings.Certificates = dm.getCertsInUse()
	return nil
}

func getAllDomains(cfg *pb.Config) (map[string]struct{}, error) {
	domains := make(map[string]struct{})
	for _, r := range cfg.Routes {
		u, err := url.Parse(r.From)
		if err != nil {
			return nil, fmt.Errorf("cannot parse from=%s: %w", r.From, err)
		}
		domains[u.Hostname()] = struct{}{}
	}
	if cfg.Settings != nil && cfg.Settings.AuthenticateServiceUrl != nil {
		u, err := url.Parse(*cfg.Settings.AuthenticateServiceUrl)
		if err != nil {
			return nil, fmt.Errorf("cannot parse authenticate_service_url=%s: %w", *cfg.Settings.AuthenticateServiceUrl, err)
		}

		domains[u.Hostname()] = struct{}{}
	}
	return domains, nil
}

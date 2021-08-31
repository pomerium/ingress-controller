package pomerium

import (
	"fmt"
	"net/url"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

// upsertCerts updates certificate bundle
func upsertCerts(cfg *pb.Config, ic *model.IngressConfig) error {
	certs, err := ic.ParseTLSCerts()
	if err != nil {
		return err
	}

	if cfg.Settings == nil {
		cfg.Settings = new(pb.Settings)
	}

	for _, cert := range certs {
		cfg.Settings.Certificates = append(cfg.Settings.Certificates, &pb.Settings_Certificate{
			CertBytes: cert.Cert,
			KeyBytes:  cert.Key,
		})
	}

	return removeUnusedCerts(cfg)
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
			return nil, fmt.Errorf("cannot parse from=%q: %w", r.From, err)
		}
		domains[u.Hostname()] = struct{}{}
	}
	return domains, nil
}

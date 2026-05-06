package certificate

import (
	"crypto/x509"
	"encoding/pem"
	"iter"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/go-set/v3"

	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// GetNamesFromConfig gets all the certificate and route host names from
// config.
func GetNamesFromConfig(
	keyPairs []*configpb.KeyPair,
	routes []*configpb.Route,
	settings []*configpb.Settings,
) (certificateNames []string, routeNames []string) {
	allCertificateNames := set.New[string](0)
	addCertificatesFromPEM := func(data []byte) {
		for c := range IterateServerCertificatesFromPEM(data) {
			if c.Subject.CommonName != "" {
				allCertificateNames.Insert(c.Subject.CommonName)
			}
			for _, n := range c.DNSNames {
				if n != "" {
					allCertificateNames.Insert(n)
				}
			}
		}
	}
	for _, kp := range keyPairs {
		addCertificatesFromPEM(kp.GetCertificate())
	}
	for _, s := range settings {
		for _, sc := range s.Certificates {
			addCertificatesFromPEM(sc.GetCertBytes())
		}
	}

	allRouteNames := set.New[string](0)
	for _, r := range routes {
		u, err := url.Parse(r.From)
		if err != nil {
			continue
		}

		// ignore ip addresses
		if _, err := netip.ParseAddr(u.Hostname()); err == nil {
			continue
		}

		// ignore non-secure URLs
		if !strings.Contains(u.Scheme, "https") {
			continue
		}

		// ignore wildcard routes
		if strings.Contains(u.Hostname(), "*") || u.Hostname() == "" {
			continue
		}

		allRouteNames.Insert(u.Hostname())
	}

	return slices.Sorted(allCertificateNames.Items()), slices.Sorted(allRouteNames.Items())
}

// IterateServerCertificatesFromPEM iterates all of the server certificates
// found in raw PEM data.
func IterateServerCertificatesFromPEM(data []byte) iter.Seq[*x509.Certificate] {
	return func(yield func(*x509.Certificate) bool) {
		for {
			var block *pem.Block
			block, data = pem.Decode(data)
			if block == nil {
				break
			}

			if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
				continue
			}

			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				continue
			}

			if !slices.Contains(cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
				continue
			}

			if !yield(cert) {
				return
			}
		}
	}
}

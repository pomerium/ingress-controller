package pomerium

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

type domainKey struct {
	Host, Domain string
}

func parseDomainKey(dnsName string) domainKey {
	parts := strings.SplitN(dnsName, ".", 2)
	if len(parts) != 2 {
		return domainKey{Host: dnsName}
	}
	return domainKey{Host: parts[0], Domain: parts[1]}
}

type certRef struct {
	inUse bool
	data  *pb.Settings_Certificate
	cert  *x509.Certificate
}

func parseCert(cert *pb.Settings_Certificate) (*x509.Certificate, error) {
	block, _ := pem.Decode(cert.CertBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode cert block")
	}

	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("expected CERTIFICATE PEM block, got %q", block.Type)
	}

	return x509.ParseCertificate(block.Bytes)
}

type domainMap map[domainKey]*certRef

func toDomainMap(certs []*pb.Settings_Certificate) (domainMap, error) {
	domains := make(domainMap)
	for _, cert := range certs {
		crt, err := parseCert(cert)
		if err != nil {
			return nil, err
		}
		domains.add(cert, crt)
	}
	return domains, nil
}

func (dm domainMap) getCertsInUse() []*pb.Settings_Certificate {
	certMap := make(map[*pb.Settings_Certificate]struct{})
	for _, ref := range dm {
		if ref.inUse {
			certMap[ref.data] = struct{}{}
		}
	}
	certs := make(byCert, 0, len(certMap))
	for crt := range certMap {
		certs = append(certs, crt)
	}
	sort.Sort(certs)
	return certs
}

func (dm domainMap) addIfNewer(key domainKey, ref *certRef) {
	cur := dm[key]
	if cur == nil {
		dm[key] = ref
		return
	}
	if cur.cert.NotAfter.Before(ref.cert.NotAfter) {
		dm[key] = ref
	}
}

func (dm domainMap) add(data *pb.Settings_Certificate, cert *x509.Certificate) {
	ref := &certRef{
		inUse: false,
		data:  data,
		cert:  cert,
	}
	for _, name := range cert.DNSNames {
		dm.addIfNewer(parseDomainKey(name), ref)
	}
}

func (dm domainMap) markInUse(dnsName string) {
	key := parseDomainKey(dnsName)
	if ref := dm[key]; ref != nil {
		ref.inUse = true
		return
	}
	if ref := dm[domainKey{Host: "*", Domain: key.Domain}]; ref != nil {
		ref.inUse = true
	}
}

type byCert []*pb.Settings_Certificate

func (a byCert) Len() int           { return len(a) }
func (a byCert) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byCert) Less(i, j int) bool { return bytes.Compare(a[i].CertBytes, a[j].CertBytes) < 0 }

package model

import (
	"fmt"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

// IngressConfig represents ingress and all other required resources
type IngressConfig struct {
	*networkingv1.Ingress
	Secrets  map[types.NamespacedName]*corev1.Secret
	Services map[types.NamespacedName]*corev1.Service
	Certs    map[types.NamespacedName]*certmanagerv1.Certificate
}

func (c *IngressConfig) UpdateDependencies(r Registry) {
	ingKey := ObjectKey(c.Ingress)
	r.DeleteCascade(ingKey)

	for _, s := range c.Secrets {
		r.Add(ingKey, ObjectKey(s))
	}
	for _, s := range c.Services {
		r.Add(ingKey, ObjectKey(s))
	}
	for _, c := range c.Certs {
		r.Add(ingKey, ObjectKey(c))
	}
}

func (ic *IngressConfig) GetServicePortByName(name types.NamespacedName, port string) (int32, error) {
	svc, ok := ic.Services[name]
	if !ok {
		return 0, fmt.Errorf("service %s was not pre-fetched, this is a bug", name.String())
	}

	for _, servicePort := range svc.Spec.Ports {
		if servicePort.Name == port {
			return servicePort.Port, nil
		}
	}

	return 0, fmt.Errorf("could not find port %s on service %s", port, name.String())
}

type TLSCert struct {
	Key  []byte
	Cert []byte
}

func (ic *IngressConfig) ParseTLSCerts() ([]*TLSCert, error) {
	certs := make([]*TLSCert, 0, len(ic.Ingress.Spec.TLS))

	for _, tls := range ic.Ingress.Spec.TLS {
		secret := ic.Secrets[types.NamespacedName{Namespace: ic.Ingress.Namespace, Name: tls.SecretName}]
		if secret == nil {
			return nil, fmt.Errorf("tls.secretName=%s, but the secret wasn't fetched. this is a bug", tls.SecretName)
		}
		if secret.Type != corev1.SecretTypeTLS {
			return nil, fmt.Errorf("tls.secretName=%s, expected type %s, got %s", tls.SecretName, corev1.SecretTypeTLS, secret.Type)
		}
		certs = append(certs, &TLSCert{
			Key:  secret.Data[corev1.TLSPrivateKeyKey],
			Cert: secret.Data[corev1.TLSCertKey],
		})
	}

	return certs, nil
}

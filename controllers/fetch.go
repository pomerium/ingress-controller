package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TLSSecret struct {
	Cert []byte
	Key  []byte
}

func fetchIngress(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) (
	*networkingv1.Ingress,
	[]*TLSSecret,
	map[types.NamespacedName]*corev1.Service,
	error,
) {
	ing := new(networkingv1.Ingress)
	if err := c.Get(ctx, namespacedName, ing); err != nil {
		return nil, nil, nil, fmt.Errorf("get %s: %w", namespacedName.String(), err)
	}

	tlsSecrets, err := fetchIngressTLS(ctx, c, namespacedName.Namespace, ing.Spec.TLS)
	if err != nil {
		// do not expose not found error
		return nil, nil, nil, fmt.Errorf("tls: %s", err.Error())
	}

	svc, err := fetchIngressServices(ctx, c, namespacedName.Namespace, ing)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("services: %s", err.Error())
	}
	return ing, tlsSecrets, svc, nil
}

// fetchIngressServices returns list of services referred from named port in the ingress path backend spec
func fetchIngressServices(ctx context.Context, c client.Client, namespace string, ing *networkingv1.Ingress) (map[types.NamespacedName]*corev1.Service, error) {
	sm := make(map[types.NamespacedName]*corev1.Service)
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			svc := p.Backend.Service
			if (svc == nil) || (svc.Port.Name == "") {
				continue
			}
			service := new(corev1.Service)
			name := types.NamespacedName{Name: svc.Name, Namespace: namespace}
			if err := c.Get(ctx, name, service); err != nil {
				return nil, fmt.Errorf("rule host=%s path=%s refers to service %s.%s port %s, failed to get service information: %w",
					rule.Host, p.Path, namespace, svc.Name, svc.Port.Name, err)
			}
			sm[name] = service
		}
	}
	return sm, nil
}

func fetchIngressTLS(ctx context.Context, c client.Client, namespace string, ingressTLS []networkingv1.IngressTLS) ([]*TLSSecret, error) {
	var secrets []*TLSSecret
	for _, tls := range ingressTLS {
		secret := new(corev1.Secret)
		name := types.NamespacedName{Namespace: namespace, Name: tls.SecretName}
		if err := c.Get(ctx, name, secret); err != nil {
			return nil, fmt.Errorf("get secret %s: %w", name.String(), err)
		}
		tlsSecret, err := parseTLSSecret(secret)
		if err != nil {
			return nil, fmt.Errorf("parsing secret %s: %w", name.String(), err)
		}
		secrets = append(secrets, tlsSecret)
	}
	return secrets, nil
}

func parseTLSSecret(secret *corev1.Secret) (*TLSSecret, error) {
	if secret.Type != corev1.SecretTypeTLS {
		return nil, fmt.Errorf("expected type %s, got %s", corev1.SecretTypeTLS, secret.Type)
	}
	return &TLSSecret{
		Key:  secret.Data[corev1.TLSPrivateKeyKey],
		Cert: secret.Data[corev1.TLSCertKey],
	}, nil
}

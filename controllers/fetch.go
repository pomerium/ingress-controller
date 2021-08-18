package controllers

import (
	"context"
	"errors"
	"fmt"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pomerium/ingress-controller/model"
)

func fetchIngress(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) (*model.IngressConfig, error) {
	ing := new(networkingv1.Ingress)
	if err := c.Get(ctx, namespacedName, ing); err != nil {
		return nil, fmt.Errorf("get %s: %w", namespacedName.String(), err)
	}

	secrets, certs, err := fetchIngressSecrets(ctx, c, namespacedName.Namespace, ing)
	if err != nil {
		// do not expose not found error
		return nil, fmt.Errorf("tls: %s", err.Error())
	}

	svc, err := fetchIngressServices(ctx, c, namespacedName.Namespace, ing)
	if err != nil {
		return nil, fmt.Errorf("services: %s", err.Error())
	}
	return &model.IngressConfig{
		Ingress:  ing,
		Secrets:  secrets,
		Services: svc,
		Certs:    certs,
	}, nil
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

func fetchIngressSecrets(ctx context.Context, c client.Client, namespace string, ingress *networkingv1.Ingress) (
	map[types.NamespacedName]*corev1.Secret,
	map[types.NamespacedName]*certmanagerv1.Certificate,
	error,
) {
	isCM := isCertManager(ingress)
	secrets := make(map[types.NamespacedName]*corev1.Secret)
	certs := make(map[types.NamespacedName]*certmanagerv1.Certificate)
	for _, tls := range ingress.Spec.TLS {
		if tls.SecretName == "" {
			return nil, nil, errors.New("tls.secretName is mandatory")
		}
		secret := new(corev1.Secret)
		name := types.NamespacedName{Namespace: namespace, Name: tls.SecretName}

		if isCM {
			// cert-manager treats Ingress.tls.secretName as Certificate, not a secret name
			// thus we need fetch Certificate first to learn an actual name of a secret
			cert := new(certmanagerv1.Certificate)
			if err := c.Get(ctx, name, cert); err != nil {
				return nil, nil, fmt.Errorf("this ingress certs are managed by cert-manager, fetching Certificate designated by tls.secretName %s: %w", name.String(), err)
			}
			certs[name] = cert
			name.Name = cert.Spec.SecretName
		}

		if err := c.Get(ctx, name, secret); err != nil {
			return nil, nil, fmt.Errorf("get secret %s: %w", name.String(), err)
		}
		secrets[name] = secret
	}
	return secrets, certs, nil
}

func isCertManager(ingress *networkingv1.Ingress) bool {
	for _, a := range []string{
		"cert-manager.io/issuer",
		"cert-manager.io/cluster-issuer",
	} {
		if _, there := ingress.Annotations[a]; there {
			return true
		}
	}
	return false
}

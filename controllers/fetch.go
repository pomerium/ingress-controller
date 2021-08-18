package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func fetchIngress(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) (
	*networkingv1.Ingress,
	[]*corev1.Secret,
	map[types.NamespacedName]*corev1.Service,
	error,
) {
	ing := new(networkingv1.Ingress)
	if err := c.Get(ctx, namespacedName, ing); err != nil {
		return nil, nil, nil, fmt.Errorf("get %s: %w", namespacedName.String(), err)
	}

	secrets, err := fetchIngressSecrets(ctx, c, namespacedName.Namespace, ing.Spec.TLS)
	if err != nil {
		// do not expose not found error
		return nil, nil, nil, fmt.Errorf("tls: %s", err.Error())
	}

	svc, err := fetchIngressServices(ctx, c, namespacedName.Namespace, ing)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("services: %s", err.Error())
	}
	return ing, secrets, svc, nil
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

func fetchIngressSecrets(ctx context.Context, c client.Client, namespace string, ingressTLS []networkingv1.IngressTLS) ([]*corev1.Secret, error) {
	var secrets []*corev1.Secret
	for _, tls := range ingressTLS {
		secret := new(corev1.Secret)
		name := types.NamespacedName{Namespace: namespace, Name: tls.SecretName}
		if err := c.Get(ctx, name, secret); err != nil {
			return nil, fmt.Errorf("get secret %s: %w", name.String(), err)
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}

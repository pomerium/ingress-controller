package controllers

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
)

func (r *Controller) fetchIngress(
	ctx context.Context,
	namespacedName types.NamespacedName,
) (*model.IngressConfig, error) {
	ingress := new(networkingv1.Ingress)
	if err := r.Client.Get(ctx, namespacedName, ingress); err != nil {
		return nil, fmt.Errorf("get %s: %w", namespacedName.String(), err)
	}

	secrets, err := r.fetchIngressSecrets(ctx, namespacedName.Namespace, ingress)
	if err != nil {
		return nil, fmt.Errorf("tls: %w", err)
	}

	svc, err := r.fetchIngressServices(ctx, namespacedName.Namespace, ingress)
	if err != nil {
		return nil, fmt.Errorf("services: %w", err)
	}
	return &model.IngressConfig{
		Ingress:  ingress,
		Secrets:  secrets,
		Services: svc,
	}, nil
}

// fetchIngressServices returns list of services referred from named port in the ingress path backend spec
func (r *Controller) fetchIngressServices(ctx context.Context, namespace string, ingress *networkingv1.Ingress) (map[types.NamespacedName]*corev1.Service, error) {
	sm := make(map[types.NamespacedName]*corev1.Service)
	for _, rule := range ingress.Spec.Rules {
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
			if err := r.Client.Get(ctx, name, service); err != nil {
				if apierrors.IsNotFound(err) {
					r.Registry.Add(r.objectKey(ingress), model.Key{Kind: r.serviceKind, NamespacedName: name})
				}

				return nil, fmt.Errorf("rule host=%s path=%s refers to service %s.%s port %s, failed to get service information: %w",
					rule.Host, p.Path, namespace, svc.Name, svc.Port.Name, err)
			}
			sm[name] = service
		}
	}
	return sm, nil
}

func (r *Controller) fetchIngressSecrets(ctx context.Context, namespace string, ingress *networkingv1.Ingress) (
	map[types.NamespacedName]*corev1.Secret,
	error,
) {
	secrets := make(map[types.NamespacedName]*corev1.Secret)
	for _, tls := range ingress.Spec.TLS {
		if tls.SecretName == "" {
			return nil, errors.New("tls.secretName is mandatory")
		}
		secret := new(corev1.Secret)
		name := types.NamespacedName{Namespace: namespace, Name: tls.SecretName}

		if err := r.Client.Get(ctx, name, secret); err != nil {
			if apierrors.IsNotFound(err) {
				r.Registry.Add(r.objectKey(ingress), model.Key{Kind: r.secretKind, NamespacedName: name})
			}
			return nil, fmt.Errorf("get secret %s: %w", name.String(), err)
		}
		secrets[name] = secret
	}
	return secrets, nil
}

package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
)

func (r *ingressController) fetchIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
) (*model.IngressConfig, error) {
	secrets, err := r.fetchIngressSecrets(ctx, ingress)
	if err != nil {
		return nil, fmt.Errorf("tls: %w", err)
	}

	svc, err := r.fetchIngressServices(ctx, ingress)
	if err != nil {
		return nil, fmt.Errorf("services: %w", err)
	}
	return &model.IngressConfig{
		AnnotationPrefix: r.annotationPrefix,
		Ingress:          ingress,
		Secrets:          secrets,
		Services:         svc,
	}, nil
}

// fetchIngressServices returns list of services referred from named port in the ingress path backend spec
func (r *ingressController) fetchIngressServices(ctx context.Context, ingress *networkingv1.Ingress) (map[types.NamespacedName]*corev1.Service, error) {
	sm := make(map[types.NamespacedName]*corev1.Service)
	ingressKey := r.objectKey(ingress)
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			svc := p.Backend.Service
			if svc == nil {
				return nil, fmt.Errorf("rule host=%s path=%s has no backend service defined", rule.Host, p.Path)
			}
			svcName := types.NamespacedName{Name: svc.Name, Namespace: ingress.Namespace}
			if err := r.fetchIngressService(ctx, ingressKey, sm, svcName); err != nil {
				return nil, fmt.Errorf("rule host=%s path=%s refers to service %s port=%s, failed to get service information: %w",
					rule.Host, p.Path, svcName.String(), svc.Port.String(), err)
			}

		}
	}

	if ingress.Spec.DefaultBackend == nil {
		return sm, nil
	}

	if err := r.fetchIngressService(ctx, ingressKey, sm, types.NamespacedName{Name: ingress.Spec.DefaultBackend.Service.Name, Namespace: ingress.Namespace}); err != nil {
		return nil, fmt.Errorf("defaultBackend: %w", err)
	}

	return sm, nil
}

func (r *ingressController) fetchIngressService(ctx context.Context, ingressKey model.Key, dst map[types.NamespacedName]*corev1.Service, name types.NamespacedName) error {
	service := new(corev1.Service)
	if err := r.Client.Get(ctx, name, service); err != nil {
		if apierrors.IsNotFound(err) {
			r.Registry.Add(ingressKey, model.Key{Kind: r.serviceKind, NamespacedName: name})
		}
		return err
	}
	dst[name] = service
	return nil
}

func (r *ingressController) fetchIngressSecrets(ctx context.Context, ingress *networkingv1.Ingress) (
	map[types.NamespacedName]*corev1.Secret,
	error,
) {
	secrets := make(map[types.NamespacedName]*corev1.Secret)
	for _, name := range r.allIngressSecrets(ingress) {
		secret := new(corev1.Secret)
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

func (r *ingressController) allIngressSecrets(ingress *networkingv1.Ingress) []types.NamespacedName {
	var names []types.NamespacedName
	for _, tls := range ingress.Spec.TLS {
		names = append(names, types.NamespacedName{Name: tls.SecretName, Namespace: ingress.Namespace})
	}
	for _, k := range []string{
		model.TLSClientSecret,
		model.TLSCustomCASecret,
		model.TLSDownstreamClientCASecret,
	} {
		key := fmt.Sprintf("%s/%s", r.annotationPrefix, k)
		if secret := ingress.Annotations[key]; secret != "" {
			names = append(names, types.NamespacedName{Name: secret, Namespace: ingress.Namespace})
		}
	}
	return names
}

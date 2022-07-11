package ingress

import (
	"context"
	"fmt"
	"strings"

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

	services, endpoints, err := r.fetchIngressServices(ctx, ingress)
	if err != nil {
		return nil, fmt.Errorf("services: %w", err)
	}

	return &model.IngressConfig{
		AnnotationPrefix: r.annotationPrefix,
		Ingress:          ingress,
		Endpoints:        endpoints,
		Secrets:          secrets,
		Services:         services,
	}, nil
}

// fetchIngressServices returns list of services referred from named port in the ingress path backend spec
func (r *ingressController) fetchIngressServices(ctx context.Context, ingress *networkingv1.Ingress) (
	map[types.NamespacedName]*corev1.Service,
	map[types.NamespacedName]*corev1.Endpoints,
	error,
) {
	sm := make(map[types.NamespacedName]*corev1.Service)
	em := make(map[types.NamespacedName]*corev1.Endpoints)
	ingressKey := model.ObjectKey(ingress, r.Scheme)
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			svc := p.Backend.Service
			if svc == nil {
				return nil, nil, fmt.Errorf("rule host=%s path=%s has no backend service defined", rule.Host, p.Path)
			}
			svcName := types.NamespacedName{Name: svc.Name, Namespace: ingress.Namespace}
			if err := r.fetchIngressService(ctx, ingressKey, sm, em, svcName); err != nil {
				return nil, nil, fmt.Errorf("rule host=%s path=%s refers to service %s port=%s, failed to get service information: %w",
					rule.Host, p.Path, svcName.String(), svc.Port.String(), err)
			}
		}
	}

	if ingress.Spec.DefaultBackend == nil {
		return sm, em, nil
	}

	if err := r.fetchIngressService(ctx, ingressKey, sm, em,
		types.NamespacedName{
			Name:      ingress.Spec.DefaultBackend.Service.Name,
			Namespace: ingress.Namespace,
		}); err != nil {
		return nil, nil, fmt.Errorf("defaultBackend: %w", err)
	}

	return sm, em, nil
}

func (r *ingressController) fetchIngressService(
	ctx context.Context,
	ingressKey model.Key,
	servicesDst map[types.NamespacedName]*corev1.Service,
	endpointsDst map[types.NamespacedName]*corev1.Endpoints,
	name types.NamespacedName,
) error {
	service := new(corev1.Service)
	if err := r.Client.Get(ctx, name, service); err != nil {
		if apierrors.IsNotFound(err) {
			r.Registry.Add(ingressKey, model.Key{Kind: r.serviceKind, NamespacedName: name})
		}
		return err
	}
	servicesDst[name] = service

	if service.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}

	endpoint := new(corev1.Endpoints)
	if err := r.Client.Get(ctx, name, endpoint); err != nil {
		if apierrors.IsNotFound(err) {
			r.Registry.Add(ingressKey, model.Key{Kind: r.endpointsKind, NamespacedName: name})
		}
		return err
	}
	endpointsDst[name] = endpoint

	return nil
}

func (r *ingressController) fetchIngressSecrets(ctx context.Context, ingress *networkingv1.Ingress) (
	map[types.NamespacedName]*corev1.Secret,
	error,
) {
	secrets := make(map[types.NamespacedName]*corev1.Secret)
	for _, name := range r.getIngressSecrets(ingress) {
		secret := new(corev1.Secret)
		if err := r.Client.Get(ctx, name, secret); err != nil {
			if apierrors.IsNotFound(err) {
				r.Registry.Add(model.ObjectKey(ingress, r.Scheme), model.Key{Kind: r.secretKind, NamespacedName: name})
			}
			return nil, fmt.Errorf("get secret %s: %w", name.String(), err)
		}
		secrets[name] = secret
	}

	return secrets, nil
}

func (r *ingressController) getIngressSecrets(ingress *networkingv1.Ingress) []types.NamespacedName {
	var names []types.NamespacedName
	for _, tls := range ingress.Spec.TLS {
		if tls.SecretName == "" {
			continue
		}
		names = append(names, types.NamespacedName{Name: tls.SecretName, Namespace: ingress.Namespace})
	}
	for key, secret := range ingress.Annotations {
		if strings.HasPrefix(key, r.annotationPrefix) && strings.HasSuffix(key, "_secret") {
			names = append(names, types.NamespacedName{Name: secret, Namespace: ingress.Namespace})
		}
	}
	return names
}

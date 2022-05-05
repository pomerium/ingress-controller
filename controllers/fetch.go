package controllers

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
	ingressKey := r.objectKey(ingress)
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
	names, expectsDefault := r.allIngressSecrets(ingress)
	for _, name := range names {
		secret := new(corev1.Secret)
		if err := r.Client.Get(ctx, name, secret); err != nil {
			if apierrors.IsNotFound(err) {
				r.Registry.Add(r.objectKey(ingress), model.Key{Kind: r.secretKind, NamespacedName: name})
			}
			return nil, fmt.Errorf("get secret %s: %w", name.String(), err)
		}
		secrets[name] = secret
	}

	if !expectsDefault || r.disableCertCheck || model.IsHTTP01Solver(ingress) {
		return secrets, nil
	}

	defaultCertSecret, err := r.fetchDefaultCert(ctx, ingress)
	if err != nil {
		return nil, fmt.Errorf("spec.TLS.secretName was empty, could not get default cert from ingressClass: %w", err)
	}
	name := types.NamespacedName{
		Namespace: defaultCertSecret.Namespace,
		Name:      defaultCertSecret.Name,
	}
	secrets[name] = defaultCertSecret
	r.Registry.Add(r.objectKey(ingress), model.Key{Kind: r.secretKind, NamespacedName: name})

	return secrets, nil
}

func (r *ingressController) allIngressSecrets(ingress *networkingv1.Ingress) ([]types.NamespacedName, bool) {
	expectsDefault := len(ingress.Spec.TLS) == 0
	var names []types.NamespacedName
	for _, tls := range ingress.Spec.TLS {
		if tls.SecretName == "" {
			expectsDefault = true
			continue
		}
		names = append(names, types.NamespacedName{Name: tls.SecretName, Namespace: ingress.Namespace})
	}
	for key, secret := range ingress.Annotations {
		if strings.HasPrefix(key, r.annotationPrefix) && strings.HasSuffix(key, "_secret") {
			names = append(names, types.NamespacedName{Name: secret, Namespace: ingress.Namespace})
		}
	}
	return names, expectsDefault
}

func (r *ingressController) fetchDefaultCert(ctx context.Context, ingress *networkingv1.Ingress) (*corev1.Secret, error) {
	class, err := r.getManagingClass(ctx, ingress)
	if err != nil {
		return nil, fmt.Errorf("could not find a matching ingressClass: %w", err)
	}

	name, err := getDefaultCertSecretName(class, r.annotationPrefix)
	if err != nil {
		return nil, fmt.Errorf("default cert secret name: %w", err)
	}

	var secret corev1.Secret
	if err := r.Client.Get(ctx, *name, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			r.Registry.Add(r.objectKey(ingress), model.Key{Kind: r.secretKind, NamespacedName: *name})
		}
		return nil, err
	}

	return &secret, nil
}

package ingress

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/ingress-controller/controllers/deps"
	"github.com/pomerium/ingress-controller/model"
)

func (r *ingressController) fetchIngress(ctx context.Context, ingress *networkingv1.Ingress) (*model.IngressConfig, error) {
	key := model.ObjectKey(ingress, r.Scheme)
	r.DeleteCascade(key)
	defer func() {
		log.FromContext(ctx).V(1).Info("current dependencies", "deps", r.Deps(key))
	}()

	client := deps.NewClient(r.Client, r.Registry, key)

	if r.updateStatusFromService != nil {
		_ = client.Get(ctx, *r.updateStatusFromService, new(corev1.Service))
	}

	return fetchIngress(ctx, client, ingress, r.annotationPrefix)
}

func fetchIngress(
	ctx context.Context,
	client client.Client,
	ingress *networkingv1.Ingress,
	annotationPrefix string,
) (*model.IngressConfig, error) {
	secrets, err := fetchIngressSecrets(ctx, client, ingress, annotationPrefix)
	if err != nil {
		return nil, fmt.Errorf("tls: %w", err)
	}

	services, endpoints, err := fetchIngressServices(ctx, client, ingress)
	if err != nil {
		return nil, fmt.Errorf("services: %w", err)
	}

	return &model.IngressConfig{
		AnnotationPrefix: annotationPrefix,
		Ingress:          ingress,
		Endpoints:        endpoints,
		Secrets:          secrets,
		Services:         services,
	}, nil
}

// fetchIngressServices returns list of services referred from named port in the ingress path backend spec
func fetchIngressServices(ctx context.Context, client client.Client, ingress *networkingv1.Ingress) (
	map[types.NamespacedName]*corev1.Service,
	map[types.NamespacedName]*corev1.Endpoints,
	error,
) {
	sm := make(map[types.NamespacedName]*corev1.Service)
	em := make(map[types.NamespacedName]*corev1.Endpoints)

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
			if err := fetchIngressService(ctx, client, sm, em, svcName); err != nil {
				return nil, nil, fmt.Errorf("rule host=%s path=%s refers to service %s port=%s, failed to get service information: %w",
					rule.Host, p.Path, svcName.String(), svc.Port.String(), err)
			}
		}
	}

	if ingress.Spec.DefaultBackend == nil {
		return sm, em, nil
	}

	if err := fetchIngressService(ctx, client, sm, em,
		types.NamespacedName{
			Name:      ingress.Spec.DefaultBackend.Service.Name,
			Namespace: ingress.Namespace,
		}); err != nil {
		return nil, nil, fmt.Errorf("defaultBackend: %w", err)
	}

	return sm, em, nil
}

func fetchIngressService(
	ctx context.Context,
	client client.Client,
	servicesDst map[types.NamespacedName]*corev1.Service,
	endpointsDst map[types.NamespacedName]*corev1.Endpoints,
	name types.NamespacedName,
) error {
	service := new(corev1.Service)
	if err := client.Get(ctx, name, service); err != nil {
		return err
	}
	servicesDst[name] = service

	if service.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}

	endpoint := new(corev1.Endpoints)
	if err := client.Get(ctx, name, endpoint); err != nil {
		return err
	}
	endpointsDst[name] = endpoint

	return nil
}

func fetchIngressSecrets(ctx context.Context, client client.Client, ingress *networkingv1.Ingress, annotationPrefix string) (
	map[types.NamespacedName]*corev1.Secret,
	error,
) {
	secrets := make(map[types.NamespacedName]*corev1.Secret)
	for _, name := range getIngressSecrets(annotationPrefix, ingress) {
		secret := new(corev1.Secret)
		if err := client.Get(ctx, name, secret); err != nil {
			return nil, fmt.Errorf("get secret %s: %w", name.String(), err)
		}
		secrets[name] = secret
	}

	return secrets, nil
}

func getIngressSecrets(annotationPrefix string, ingress *networkingv1.Ingress) []types.NamespacedName {
	var names []types.NamespacedName
	for _, tls := range ingress.Spec.TLS {
		if tls.SecretName == "" {
			continue
		}
		names = append(names, types.NamespacedName{Name: tls.SecretName, Namespace: ingress.Namespace})
	}
	for key, secret := range ingress.Annotations {
		if strings.HasPrefix(key, annotationPrefix) && strings.HasSuffix(key, "_secret") {
			names = append(names, types.NamespacedName{Name: secret, Namespace: ingress.Namespace})
		}
	}
	return names
}

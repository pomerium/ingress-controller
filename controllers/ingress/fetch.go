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

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
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

	return FetchIngress(ctx, client, ingress, r.annotationPrefix)
}

// FetchIngress populates a model.IngressConfig for ingress.
func FetchIngress(
	ctx context.Context,
	client client.Client,
	ingress *networkingv1.Ingress,
	annotationPrefix string,
) (*model.IngressConfig, error) {
	secrets, err := fetchIngressSecrets(ctx, client, ingress, annotationPrefix)
	if err != nil {
		return nil, fmt.Errorf("tls: %w", err)
	}

	services, endpoints, pomeriumServices, err := fetchIngressServices(ctx, client, ingress)
	if err != nil {
		return nil, fmt.Errorf("services: %w", err)
	}

	return &model.IngressConfig{
		AnnotationPrefix: annotationPrefix,
		Ingress:          ingress,
		Endpoints:        endpoints,
		Secrets:          secrets,
		Services:         services,
		PomeriumServices: pomeriumServices,
	}, nil
}

// fetchIngressServices returns the services and PomeriumServices the ingress
// backends refer to
func fetchIngressServices(ctx context.Context, client client.Client, ingress *networkingv1.Ingress) (
	map[types.NamespacedName]*corev1.Service,
	map[types.NamespacedName]*corev1.Endpoints,
	map[types.NamespacedName]*icsv1.PomeriumService,
	error,
) {
	sm := make(map[types.NamespacedName]*corev1.Service)
	em := make(map[types.NamespacedName]*corev1.Endpoints)
	psm := make(map[types.NamespacedName]*icsv1.PomeriumService)

	fetchBackend := func(backend networkingv1.IngressBackend) error {
		if res := backend.Resource; backend.Service == nil && res != nil {
			if !model.IsPomeriumServiceRef(res) {
				return fmt.Errorf("unsupported resource backend %s", model.ResourceRefString(res))
			}
			name := types.NamespacedName{Name: res.Name, Namespace: ingress.Namespace}
			ps := new(icsv1.PomeriumService)
			if err := client.Get(ctx, name, ps); err != nil {
				return fmt.Errorf("get PomeriumService %s: %w", name.String(), err)
			}
			psm[name] = ps
			return nil
		}
		svc := backend.Service
		if svc == nil {
			return fmt.Errorf("no backend service defined")
		}
		svcName := types.NamespacedName{Name: svc.Name, Namespace: ingress.Namespace}
		if err := fetchIngressService(ctx, client, sm, em, svcName); err != nil {
			return fmt.Errorf("refers to service %s port=%s, failed to get service information: %w",
				svcName.String(), svc.Port.String(), err)
		}
		return nil
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			if err := fetchBackend(p.Backend); err != nil {
				return nil, nil, nil, fmt.Errorf("rule host=%s path=%s: %w", rule.Host, p.Path, err)
			}
		}
	}

	if ingress.Spec.DefaultBackend == nil {
		return sm, em, psm, nil
	}

	if err := fetchBackend(*ingress.Spec.DefaultBackend); err != nil {
		return nil, nil, nil, fmt.Errorf("defaultBackend: %w", err)
	}

	return sm, em, psm, nil
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

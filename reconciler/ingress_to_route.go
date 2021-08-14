package reconciler

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/gosimple/slug"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

type serviceMap map[types.NamespacedName]*corev1.Service

func (sm serviceMap) getPortByName(name types.NamespacedName, port string) (int32, error) {
	svc, ok := sm[name]
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

// ingressToRoute converts Ingress object into Pomerium Route
func ingressToRoute(ing *networkingv1.Ingress, sm serviceMap) (routeList, error) {
	tmpl := &pb.Route{}

	if err := applyAnnotations(tmpl, ing.Annotations, "ingress.pomerium.io"); err != nil {
		return nil, fmt.Errorf("annotations: %w", err)
	}

	routes := make(routeList, 0, len(ing.Spec.Rules))
	for _, rule := range ing.Spec.Rules {
		r, err := ruleToRoute(rule, tmpl, types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name}, sm)
		if err != nil {
			return nil, err
		}
		routes = append(routes, r...)
	}

	return routes, nil
}

func ruleToRoute(rule networkingv1.IngressRule, tmpl *pb.Route, name types.NamespacedName, sm serviceMap) ([]*pb.Route, error) {
	if rule.Host == "" {
		return nil, errors.New("host is required")
	}

	if rule.HTTP == nil {
		return nil, errors.New("rules.http is required")
	}

	routes := make(routeList, 0, len(rule.HTTP.Paths))
	for _, p := range rule.HTTP.Paths {
		r := proto.Clone(tmpl).(*pb.Route)
		r.From = (&url.URL{Scheme: "https", Host: rule.Host}).String()
		if err := pathToRoute(r, name, p, sm); err != nil {
			return nil, err
		}
		routes = append(routes, r)
	}

	// https://kubernetes.io/docs/concepts/services-networking/ingress/#multiple-matches
	// envoy matches according to the order routes are present in the configuration
	routes.Sort()

	return routes, nil
}

func pathToRoute(r *pb.Route, name types.NamespacedName, p networkingv1.HTTPIngressPath, sm serviceMap) error {
	// https://kubernetes.io/docs/concepts/services-networking/ingress/#path-types
	// Paths that do not include an explicit pathType will fail validation.
	if p.PathType == nil {
		return fmt.Errorf("pathType is required")
	}

	switch *p.PathType {
	case networkingv1.PathTypeImplementationSpecific:
		return fmt.Errorf("pathType %s unsupported, please explicitly choose between %s or %s",
			networkingv1.PathTypeImplementationSpecific,
			networkingv1.PathTypePrefix,
			networkingv1.PathTypeExact,
		)
	case networkingv1.PathTypePrefix:
		r.Prefix = p.Path
	case networkingv1.PathTypeExact:
		r.Path = p.Path
	default:
		// shouldn't get there as apiserver should not allow this
		return fmt.Errorf("unknown pathType %s", *p.PathType)
	}

	setRouteNameID(r, name, p.Path)

	svcURL, err := getServiceURL(name.Namespace, p, sm)
	if err != nil {
		return fmt.Errorf("backend: %w", err)
	}

	r.To = []string{svcURL.String()}
	return nil
}

func setRouteNameID(r *pb.Route, name types.NamespacedName, path string) error {
	id, err := (&routeID{Name: name.Name, Namespace: name.Namespace, Path: path}).Marshal()
	if err != nil {
		return err
	}
	r.Id = id

	r.Name = fmt.Sprintf("%s-%s", name.Namespace, name.Name)
	pathSlug := slug.Make(path)
	if pathSlug != "" {
		r.Name = fmt.Sprintf("%s-%s", r.Name, pathSlug)
	}

	return nil
}

func getServiceURL(namespace string, p networkingv1.HTTPIngressPath, sm serviceMap) (*url.URL, error) {
	svc := p.Backend.Service
	if svc == nil {
		return nil, errors.New("service is missing")
	}

	port := svc.Port.Number
	if svc.Port.Name != "" {
		var err error
		port, err = sm.getPortByName(types.NamespacedName{Namespace: namespace, Name: svc.Name}, svc.Port.Name)
		if err != nil {
			return nil, err
		}
	}

	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s:%d", svc.Name, namespace, port),
	}, nil
}

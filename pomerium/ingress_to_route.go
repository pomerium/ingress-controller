package pomerium

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gosimple/slug"
	"google.golang.org/protobuf/proto"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

const (
	// IngressAnnotationPrefix is default prefix for Pomerium ingress controller annotations
	IngressAnnotationPrefix = "ingress.pomerium.io"
	httpSolverLabel         = "acme.cert-manager.io/http01-solver"
)

// ingressToRoutes converts Ingress object into Pomerium Route
func ingressToRoutes(ctx context.Context, ic *model.IngressConfig) (routeList, error) {
	tmpl := &pb.Route{}

	if isHTTP01Solver(ic.Ingress) {
		log.FromContext(ctx).Info("Ingress is HTTP-01 challenge solver, enabling public unauthenticated access")
		tmpl.AllowPublicUnauthenticatedAccess = true
		tmpl.PreserveHostHeader = true
	} else if err := applyAnnotations(tmpl, ic.Ingress.Annotations, IngressAnnotationPrefix); err != nil {
		return nil, fmt.Errorf("annotations: %w", err)
	}

	routes := make(routeList, 0, len(ic.Ingress.Spec.Rules))
	for _, rule := range ic.Ingress.Spec.Rules {
		r, err := ruleToRoute(rule, tmpl, types.NamespacedName{Namespace: ic.Ingress.Namespace, Name: ic.Ingress.Name}, ic)
		if err != nil {
			return nil, err
		}
		routes = append(routes, r...)
	}

	return routes, nil
}

func isHTTP01Solver(ingress *networkingv1.Ingress) bool {
	return strings.ToLower(ingress.Labels[httpSolverLabel]) == "true"
}

func ruleToRoute(rule networkingv1.IngressRule, tmpl *pb.Route, name types.NamespacedName, ic *model.IngressConfig) ([]*pb.Route, error) {
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
		if err := pathToRoute(r, name, rule.Host, p, ic); err != nil {
			return nil, err
		}
		routes = append(routes, r)
	}

	// https://kubernetes.io/docs/concepts/services-networking/ingress/#multiple-matches
	// envoy matches according to the order routes are present in the configuration
	routes.Sort()

	return routes, nil
}

func pathToRoute(r *pb.Route, name types.NamespacedName, host string, p networkingv1.HTTPIngressPath, ic *model.IngressConfig) error {
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

	if err := setRouteNameID(r, name, url.URL{Host: host, Path: p.Path}); err != nil {
		return fmt.Errorf("setRouteNameID: %w", err)
	}

	svcURL, err := getServiceURL(name.Namespace, p, ic)
	if err != nil {
		return fmt.Errorf("backend: %w", err)
	}

	r.To = []string{svcURL.String()}
	return nil
}

func setRouteNameID(r *pb.Route, name types.NamespacedName, u url.URL) error {
	id, err := (&routeID{Name: name.Name, Namespace: name.Namespace, Host: u.Host, Path: u.Path}).Marshal()
	if err != nil {
		return err
	}
	r.Id = id

	r.Name = slug.Make(fmt.Sprintf("%s %s %s", name.Namespace, name.Name, u.Host))
	pathSlug := slug.Make(u.Path)
	if pathSlug != "" {
		r.Name = fmt.Sprintf("%s-%s", r.Name, pathSlug)
	}

	return nil
}

func getServiceURL(namespace string, p networkingv1.HTTPIngressPath, ic *model.IngressConfig) (*url.URL, error) {
	svc := p.Backend.Service
	if svc == nil {
		return nil, errors.New("service is missing")
	}

	port := svc.Port.Number
	if svc.Port.Name != "" {
		var err error
		port, err = ic.GetServicePortByName(types.NamespacedName{Namespace: namespace, Name: svc.Name}, svc.Port.Name)
		if err != nil {
			return nil, err
		}
	}

	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, namespace, port),
	}, nil
}

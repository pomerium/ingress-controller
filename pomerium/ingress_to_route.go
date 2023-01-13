package pomerium

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"

	"github.com/gosimple/slug"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

// ingressToRoutes converts Ingress object into Pomerium Route
func ingressToRoutes(ctx context.Context, ic *model.IngressConfig) (routeList, error) {
	tmpl := &pb.Route{}

	if model.IsHTTP01Solver(ic.Ingress) {
		log.FromContext(ctx).Info("Ingress is HTTP-01 challenge solver, enabling public unauthenticated access")
		tmpl.AllowPublicUnauthenticatedAccess = true
		tmpl.PreserveHostHeader = true
	} else if err := applyAnnotations(tmpl, ic); err != nil {
		return nil, fmt.Errorf("annotations: %w", err)
	}

	routes := make(routeList, 0, len(ic.Ingress.Spec.Rules)+1)
	if ic.Ingress.Spec.DefaultBackend != nil {
		r, err := defaultBackend(tmpl, ic)
		if err != nil {
			return nil, fmt.Errorf("defaultBackend: %w", err)
		}
		routes = append(routes, r)
	}
	for _, rule := range ic.Ingress.Spec.Rules {
		r, err := ruleToRoute(rule, tmpl, ic)
		if err != nil {
			return nil, err
		}
		routes = append(routes, r...)
	}

	return routes, nil
}

func deriveHostFromTLS(tls []networkingv1.IngressTLS) (string, error) {
	if len(tls) != 1 {
		return "", fmt.Errorf("expected one TLS spec, got %d", len(tls))
	}
	if len(tls[0].Hosts) != 1 {
		return "", fmt.Errorf("expected exactly one Host in the TLS spec, got %d", len(tls[0].Hosts))
	}
	return tls[0].Hosts[0], nil
}

func defaultBackend(tmpl *pb.Route, ic *model.IngressConfig) (*pb.Route, error) {
	host, err := deriveHostFromTLS(ic.Spec.TLS)
	if err != nil {
		return nil, fmt.Errorf("deriving host: %w", err)
	}

	typePrefix := networkingv1.PathTypePrefix
	routes, err := ruleToRoute(networkingv1.IngressRule{
		Host: host,
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					Path:     "/",
					PathType: &typePrefix,
					Backend:  *ic.Spec.DefaultBackend,
				}},
			},
		},
	}, tmpl, ic)
	if err != nil {
		return nil, err
	}
	if len(routes) != 1 {
		return nil, fmt.Errorf("expected 1 route, got %d", len(routes))
	}
	return routes[0], nil
}

func ruleToRoute(rule networkingv1.IngressRule, tmpl *pb.Route, ic *model.IngressConfig) ([]*pb.Route, error) {
	if rule.Host == "" {
		return nil, errors.New("host is required")
	}

	if rule.HTTP == nil {
		return nil, errors.New("rules.http is required")
	}

	routes := make(routeList, 0, len(rule.HTTP.Paths))
	for _, p := range rule.HTTP.Paths {
		r := proto.Clone(tmpl).(*pb.Route)
		if err := pathToRoute(r, rule.Host, p, ic); err != nil {
			return nil, fmt.Errorf("pathToRoute: %s: %w", p.String(), err)
		}
		routes = append(routes, r)
	}

	return routes, nil
}

func pathToRoute(r *pb.Route, host string, p networkingv1.HTTPIngressPath, ic *model.IngressConfig) error {
	if err := setRouteFrom(r, host, p, ic); err != nil {
		return fmt.Errorf("from: %w", err)
	}

	if err := setRoutePath(r, p, ic); err != nil {
		return fmt.Errorf("path: %w", err)
	}

	if err := setRouteNameID(r, ic.GetNamespacedName(ic.Name), url.URL{Host: host, Path: p.Path}); err != nil {
		return fmt.Errorf("name: %w", err)
	}

	if err := setServiceURLs(r, p, ic); err != nil {
		return fmt.Errorf("backend: %w", err)
	}

	return nil
}

func setRoutePath(r *pb.Route, p networkingv1.HTTPIngressPath, ic *model.IngressConfig) error {
	// https://kubernetes.io/docs/concepts/services-networking/ingress/#path-types
	// Paths that do not include an explicit pathType will fail validation.
	if p.PathType == nil {
		return fmt.Errorf("pathType is required")
	}

	if ic.IsTCPUpstream() {
		if *p.PathType != networkingv1.PathTypeImplementationSpecific {
			return fmt.Errorf("tcp services must have %s path type", networkingv1.PathTypeImplementationSpecific)
		}
		if p.Path != "" {
			return fmt.Errorf("tcp services must not specify path, got %s", r.Path)
		}
		return nil
	}

	switch *p.PathType {
	case networkingv1.PathTypeImplementationSpecific:
		if ic.IsPathRegex() {
			r.Regex = p.Path
		} else {
			r.Prefix = p.Path
		}
	case networkingv1.PathTypeExact:
		r.Path = p.Path
	case networkingv1.PathTypePrefix:
		r.Prefix = p.Path
	default:
		// shouldn't get there as api server should not allow this
		return fmt.Errorf("unknown pathType %s", *p.PathType)
	}

	return nil
}

func setRouteFrom(r *pb.Route, host string, p networkingv1.HTTPIngressPath, ic *model.IngressConfig) error {
	u := url.URL{
		Scheme: "https",
		Host:   host,
	}

	if ic.IsTCPUpstream() {
		_, _, port, err := getServiceFromPath(p, ic)
		if err != nil {
			return err
		}
		u.Host = net.JoinHostPort(u.Host, fmt.Sprint(port))
		u.Scheme = "tcp+https"
	}

	r.From = u.String()
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

func getServiceFromPath(p networkingv1.HTTPIngressPath, ic *model.IngressConfig) (*networkingv1.IngressServiceBackend, *corev1.Service, int32, error) {
	backend := p.Backend.Service
	if backend == nil {
		return nil, nil, -1, errors.New("service must be specified")
	}

	serviceName := ic.GetNamespacedName(backend.Name)
	port := backend.Port.Number
	if backend.Port.Name != "" {
		var err error
		port, err = ic.GetServicePortByName(serviceName, backend.Port.Name)
		if err != nil {
			return nil, nil, -1, fmt.Errorf("get service %s port by name %s: %w", serviceName.String(), backend.Port.Name, err)
		}
	}

	service, ok := ic.Services[serviceName]
	if !ok {
		return nil, nil, -1, fmt.Errorf("service %s was not fetched, this is a bug", serviceName.String())
	}

	return backend, service, port, nil
}

func getPathServiceHosts(r *pb.Route, p networkingv1.HTTPIngressPath, ic *model.IngressConfig) ([]string, error) {
	backend, service, port, err := getServiceFromPath(p, ic)
	if err != nil {
		return nil, fmt.Errorf("get service from path: %w", err)
	}

	var hosts []string
	if service.Spec.Type == corev1.ServiceTypeExternalName {
		hosts = append(hosts, fmt.Sprintf("%s:%d", service.Spec.ExternalName, port))
	} else if ic.UseServiceProxy() {
		hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local:%d", backend.Name, ic.Namespace, port))
	} else {
		endpoints, ok := ic.Endpoints[ic.GetNamespacedName(backend.Name)]
		if ok {
			hosts = getEndpointsURLs(backend.Port, service.Spec.Ports, endpoints.Subsets)
		}
		// this can happen if no endpoints are ready, or none match, in which case we fallback to the Kubernetes DNS name
		if len(hosts) == 0 {
			hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local:%d", backend.Name, ic.Namespace, port))
		} else if ic.IsSecureUpstream() && r.TlsServerName == "" {
			r.TlsServerName = fmt.Sprintf("%s.%s.svc.cluster.local", backend.Name, ic.Namespace)
		}
	}

	return hosts, nil
}

func getUpstreamScheme(ic *model.IngressConfig) string {
	if ic.IsTCPUpstream() {
		return "tcp"
	} else if ic.IsSecureUpstream() {
		return "https"
	}
	return "http"
}

func setServiceURLs(r *pb.Route, p networkingv1.HTTPIngressPath, ic *model.IngressConfig) error {
	hosts, err := getPathServiceHosts(r, p, ic)
	if err != nil {
		return fmt.Errorf("get service hosts: %w", err)
	}

	var urls []string
	scheme := getUpstreamScheme(ic)
	for _, host := range hosts {
		urls = append(urls, (&url.URL{
			Scheme: scheme,
			Host:   host,
		}).String())
	}
	sort.Strings(urls)

	r.To = urls
	return nil
}

func getEndpointsURLs(ingressServicePort networkingv1.ServiceBackendPort, servicePorts []corev1.ServicePort, endpointSubsets []corev1.EndpointSubset) []string {
	portMatch := getEndpointPortMatcher(ingressServicePort, servicePorts)
	if portMatch == nil {
		return nil
	}
	var hosts []string
	for _, subset := range endpointSubsets {
		for _, endpointAddress := range subset.Addresses {
			for _, endpointPort := range subset.Ports {
				if portMatch(endpointPort) {
					hosts = append(hosts, fmt.Sprintf("%s:%d", endpointAddress.IP, endpointPort.Port))
				}
			}
		}
	}
	return hosts
}

func getEndpointPortMatcher(ingressServicePort networkingv1.ServiceBackendPort, servicePorts []corev1.ServicePort) func(port corev1.EndpointPort) bool {
	if ingressServicePort.Name != "" {
		ports := make(map[intstr.IntOrString]bool)
		for _, sp := range servicePorts {
			if sp.Name == ingressServicePort.Name {
				ports[sp.TargetPort] = true
			}
		}
		return func(port corev1.EndpointPort) bool {
			pName := intstr.FromString(port.Name)
			pNumber := intstr.FromInt(int(port.Port))

			return port.Name == ingressServicePort.Name && (ports[pName] || ports[pNumber])
		}
	}

	// match by port number
	for _, sp := range servicePorts {
		if sp.Port == ingressServicePort.Number {
			return func(port corev1.EndpointPort) bool {
				return sp.TargetPort.IntVal == port.Port
			}
		}
	}

	return nil
}

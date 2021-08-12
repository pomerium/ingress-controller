package reconciler

import (
	"errors"
	"fmt"
	"net/url"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	"google.golang.org/protobuf/proto"
	networkingv1 "k8s.io/api/networking/v1"
)

// ingressToRoute converts Ingress object into Pomerium Route
func ingressToRoute(ing *networkingv1.Ingress) ([]*pb.Route, error) {
	tmpl := &pb.Route{
		Name:                      ing.Name,
		Id:                        string(ing.GetUID()),
		AllowAnyAuthenticatedUser: true,
	}

	var routes []*pb.Route
	for _, rule := range ing.Spec.Rules {
		r, err := ruleToRoute(rule, tmpl, ing.Namespace)
		if err != nil {
			return nil, err
		}
		routes = append(routes, r...)
	}

	return routes, nil
}

func ruleToRoute(rule networkingv1.IngressRule, tmpl *pb.Route, namespace string) ([]*pb.Route, error) {
	if rule.Host == "" {
		return nil, errors.New("host is required")
	}

	if rule.HTTP == nil {
		return nil, errors.New("rules.http is required")
	}

	out := make([]*pb.Route, 0, len(rule.HTTP.Paths))
	for i, p := range rule.HTTP.Paths {
		r := proto.Clone(tmpl).(*pb.Route)
		r.From = (&url.URL{Scheme: "https", Host: rule.Host}).String()
		if err := pathToRoute(r, namespace, p); err != nil {
			return nil, err
		}
		r.Name = fmt.Sprintf("%s-%d", r.Name, i)
		r.Id = fmt.Sprintf("%s-%d", r.Id, i)
		out = append(out, r)
	}
	return out, nil
}

func pathToRoute(r *pb.Route, namespace string, p networkingv1.HTTPIngressPath) error {
	if err := addTo(r, namespace, p); err != nil {
		return fmt.Errorf("backend: %w", err)
	}

	if err := addPathRules(r, p); err != nil {
		return fmt.Errorf("path rules: %w", err)
	}

	return nil
}

func addPathRules(r *pb.Route, p networkingv1.HTTPIngressPath) error {
	// TODO: implement
	return nil
}

func addTo(r *pb.Route, namespace string, p networkingv1.HTTPIngressPath) error {
	svc := p.Backend.Service
	if svc == nil {
		return errors.New("service is missing")
	}

	if svc.Port.Name != "" {
		return errors.New("port names unsupported")
	} else if svc.Port.Number <= 0 {
		return fmt.Errorf("invalid port number %d", svc.Port.Number)
	}

	// Ingress v1 only supports plaintext http destinations
	r.To = []string{(&url.URL{Scheme: "http", Host: fmt.Sprintf("%s.%s:%d", svc.Name, namespace, svc.Port.Number)}).String()}
	return nil
}

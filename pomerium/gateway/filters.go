package gateway

import (
	"fmt"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	icgv1alpha1 "github.com/pomerium/ingress-controller/apis/gateway/v1alpha1"
	"github.com/pomerium/ingress-controller/model"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/policy"
)

func applyFilters(
	route *pb.Route,
	config *model.GatewayConfig,
	routeConfig *model.GatewayHTTPRouteConfig,
	filters []gateway_v1.HTTPRouteFilter,
) error {
	for i := range filters {
		if err := applyFilter(route, config, routeConfig, &filters[i]); err != nil {
			return err
		}
	}
	return nil
}

func applyFilter(
	route *pb.Route,
	config *model.GatewayConfig,
	routeConfig *model.GatewayHTTPRouteConfig,
	filter *gateway_v1.HTTPRouteFilter,
) error {
	switch filter.Type {
	case gateway_v1.HTTPRouteFilterRequestHeaderModifier:
		applyRequestHeaderFilter(route, filter.RequestHeaderModifier)
	case gateway_v1.HTTPRouteFilterRequestRedirect:
		applyRedirectFilter(route, filter.RequestRedirect)
	case gateway_v1.HTTPRouteFilterExtensionRef:
		return applyExtensionFilter(route, config, routeConfig, filter.ExtensionRef)
	default:
		return fmt.Errorf("filter type %q not supported", filter.Type)
	}
	return nil
}

func applyRequestHeaderFilter(route *pb.Route, filter *gateway_v1.HTTPHeaderFilter) {
	// Note: "append" is not supported yet.
	route.SetRequestHeaders = makeHeadersMap(filter.Set)
	route.RemoveRequestHeaders = filter.Remove
}

func makeHeadersMap(headers []gateway_v1.HTTPHeader) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	m := make(map[string]string)
	for i := range headers {
		m[string(headers[i].Name)] = headers[i].Value
	}
	return m
}

func applyRedirectFilter(route *pb.Route, filter *gateway_v1.HTTPRequestRedirectFilter) {
	rr := pb.RouteRedirect{
		SchemeRedirect: filter.Scheme,
		HostRedirect:   (*string)(filter.Hostname),
	}
	if filter.StatusCode != nil {
		code := int32(*filter.StatusCode)
		rr.ResponseCode = &code
	}
	if filter.Port != nil {
		port := uint32(*filter.Port)
		rr.PortRedirect = &port
	}
	route.Redirect = &rr
}

func applyExtensionFilter(
	route *pb.Route,
	config *model.GatewayConfig,
	routeConfig *model.GatewayHTTPRouteConfig,
	filter *gateway_v1.LocalObjectReference,
) error {
	// Make sure the API group is the one we expect.
	if filter.Group != gateway_v1.Group(icgv1alpha1.GroupVersion.Group) {
		return fmt.Errorf("unsupported filter group %q", filter.Group)
	}

	k := model.ExtensionFilterKey{
		Kind:      string(filter.Kind),
		Namespace: routeConfig.Namespace,
		Name:      string(filter.Name),
	}
	f := config.ExtensionFilters[k]
	if f == nil {
		return fmt.Errorf("filter not found (%v)", k)
	}

	f.ApplyToRoute(route)
	return nil
}

// PolicyFilter applies a Pomerium policy defined by the PolicyFilter CRD.
type PolicyFilter struct {
	rego string
}

// NewPolicyFilter parses a PolicyFilter CRD object, returning an error if the object is not valid.
func NewPolicyFilter(obj *icgv1alpha1.PolicyFilter) (*PolicyFilter, error) {
	src, err := policy.GenerateRegoFromReader(strings.NewReader(obj.Spec.PPL))
	if err != nil {
		return nil, fmt.Errorf("couldn't parse policy: %w", err)
	}

	_, err = ast.ParseModule("policy.rego", src)
	if err != nil && strings.Contains(err.Error(), "package expected") {
		_, err = ast.ParseModule("policy.rego", "package pomerium.policy\n\n"+src)
	}
	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}
	return &PolicyFilter{src}, nil
}

// ApplyToRoute applies this policy filter to a Pomerium route proto.
func (f *PolicyFilter) ApplyToRoute(r *pb.Route) {
	r.Policies = append(r.Policies, &pb.Policy{Rego: []string{f.rego}})
}

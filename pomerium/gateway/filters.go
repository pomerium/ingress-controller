package gateway

import (
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func applyFilters(route *pb.Route, filters []gateway_v1.HTTPRouteFilter) {
	for i := range filters {
		applyFilter(route, &filters[i])
	}
}

func applyFilter(route *pb.Route, filter *gateway_v1.HTTPRouteFilter) {
	switch filter.Type {
	case gateway_v1.HTTPRouteFilterRequestHeaderModifier:
		applyRequestHeaderFilter(route, filter.RequestHeaderModifier)
	case gateway_v1.HTTPRouteFilterRequestRedirect:
		applyRedirectFilter(route, filter.RequestRedirect)
	}
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

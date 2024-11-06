package gateway

import (
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// XXX: If a reference to a custom filter type cannot be resolved, the filter MUST NOT be skipped.
// Instead, requests that would have been processed by that filter MUST receive a HTTP error response.
// --> maybe we can set a DirectResponse 503 in this case?

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
	// XXX: "append" is not supported
	route.SetRequestHeaders = makeHeadersMap(filter.Set)
	route.RemoveRequestHeaders = filter.Remove

	// XXX: should host header rewriting be supported? if so, we can't use SetRequestHeaders for it
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
	// XXX: the filter values must already have been validated
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

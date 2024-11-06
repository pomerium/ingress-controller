package gateway

import (
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func applyMatch(route *pb.Route, match *gateway_v1.HTTPRouteMatch) (ok bool) {
	if len(match.Headers) > 0 || len(match.QueryParams) > 0 || match.Method != nil {
		return false // these features are not supported yet
		// XXX: need to propagate this back into a status condition somehow
	}
	applyPathMatch(route, match.Path)
	return true
}

func applyPathMatch(route *pb.Route, match *gateway_v1.HTTPPathMatch) {
	if match == nil || match.Type == nil || match.Value == nil {
		return
	}

	switch *match.Type {
	case gateway_v1.PathMatchExact:
		route.Path = *match.Value
	case gateway_v1.PathMatchPathPrefix:
		route.Prefix = *match.Value
	case gateway_v1.PathMatchRegularExpression:
		route.Regex = *match.Value
	}
}

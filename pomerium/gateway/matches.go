package gateway

import (
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func applyMatch(route *pb.Route, match *gateway_v1.HTTPRouteMatch) {
	if match.Path != nil {
		applyPathMatch(route, match.Path)
	}
}

func applyPathMatch(route *pb.Route, match *gateway_v1.HTTPPathMatch) {
	if match.Type == nil || match.Value == nil {
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

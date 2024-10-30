package gateway

import (
	"net/url"

	"github.com/pomerium/ingress-controller/model"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	"google.golang.org/protobuf/proto"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

func GatewayRoutes(gc *model.GatewayHTTPRouteConfig) []*pb.Route {
	// A single HTTPRoute may need to be represented using many Pomerium routes:
	//  - An HTTPRoute may have multiple hostnames.
	//  - An HTTPRoute may have multiple HTTPRouteRules.
	//  - An HTTPRouteRule may have multiple
	// First we'll expand all HTTPRouteRules into "template" Pomerium routes, and then we'll
	// repeat each "template" route once per hostname.
	trs := templateRoutes(gc.HTTPRoute)

	// Special case: if no hostname is defined, the routes should match all hostnames.
	// We can represent this as a wildcard Pomerium route.
	hostnameCount := len(gc.Hostnames)
	if hostnameCount == 0 {
		for i := range trs {
			trs[i].From = "https://*"
		}
		return trs
	}

	prs := make([]*pb.Route, hostnameCount*len(trs))
	i := 0
	for _, h := range gc.Hostnames {
		from := (&url.URL{
			Scheme: "https",
			Host:   h,
		}).String()
		for j := range trs {
			prs[i] = proto.Clone(trs[j]).(*pb.Route)
			prs[i].From = from
			// XXX: set a useful Name on the route?
			i++
		}
	}

	return prs
}

// templateRoutes converts an HTTPRoute into zero or more Pomerium routes, ignoring hostname.
func templateRoutes(gc *gateway_v1.HTTPRoute) []*pb.Route {
	// XXX: error reporting for any unsupported features (maybe as a separate pass in the reconcile loop?)

	var prs []*pb.Route

	rules := gc.Spec.Rules
	for i := range rules {
		rule := &rules[i]
		pr := &pb.Route{}
		applyFilters(pr, rule.Filters)
		// XXX: figure out what to do if there are no non-zero weight backendRefs
		applyBackendRefs(pr, rule.BackendRefs)

		if len(rule.Matches) == 0 {
			prs = append(prs, pr)
			continue
		}

		for j := range rule.Matches {
			cloned := proto.Clone(pr).(*pb.Route)
			applyMatch(cloned, &rule.Matches[j])
			prs = append(prs, cloned)
		}
	}

	// XXX: There is a specific precedence to respect among overlapping routes.
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteRule:~:text=Proxy%20or%20Load%20Balancer%20routing%20configuration%20generated%20from%20HTTPRoutes,from%2C%20a%20HTTP%20404%20status%20code%20MUST%20be%20returned.

	return prs
}

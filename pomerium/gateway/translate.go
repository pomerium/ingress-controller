package gateway

import (
	"net/url"

	"github.com/pomerium/ingress-controller/model"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	"google.golang.org/protobuf/proto"
)

func GatewayRoutes(gc *model.GatewayHTTPRouteConfig) []*pb.Route {
	// A single HTTPRoute may need to be represented using many Pomerium routes:
	//  - An HTTPRoute may have multiple hostnames.
	//  - An HTTPRoute may have multiple HTTPRouteRules.
	//  - An HTTPRouteRule may have multiple HTTPRouteMatches.
	// First we'll expand all HTTPRouteRules into "template" Pomerium routes, and then we'll
	// repeat each "template" route once per hostname.
	trs := templateRoutes(gc)

	prs := make([]*pb.Route, len(gc.Hostnames)*len(trs))
	i := 0
	for _, h := range gc.Hostnames {
		from := (&url.URL{
			Scheme: "https",
			Host:   string(h),
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
func templateRoutes(gc *model.GatewayHTTPRouteConfig) []*pb.Route {
	// XXX: error reporting for any unsupported features (maybe as a separate pass in the reconcile loop?)

	var prs []*pb.Route

	rules := gc.Spec.Rules
	for i := range rules {
		rule := &rules[i]
		pr := &pb.Route{}

		// XXX -- for testing, apply a public access policy
		// DO NOT MERGE
		pr.AllowPublicUnauthenticatedAccess = true

		// From the spec (near https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPRoute):
		// "Implementations MUST ignore any port value specified in the HTTP Host header while
		// performing a match and (absent of any applicable header modification configuration) MUST
		// forward this header unmodified to the backend."
		pr.PreserveHostHeader = true

		applyFilters(pr, rule.Filters)
		applyBackendRefs(pr, gc, rule.BackendRefs)

		if len(rule.Matches) == 0 {
			prs = append(prs, pr)
			continue
		}

		for j := range rule.Matches {
			cloned := proto.Clone(pr).(*pb.Route)
			if applyMatch(cloned, &rule.Matches[j]) {
				prs = append(prs, cloned)
			}
		}
	}

	// XXX: There is a specific precedence to respect among overlapping routes.
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteRule:~:text=Proxy%20or%20Load%20Balancer%20routing%20configuration%20generated%20from%20HTTPRoutes,from%2C%20a%20HTTP%20404%20status%20code%20MUST%20be%20returned.

	return prs
}

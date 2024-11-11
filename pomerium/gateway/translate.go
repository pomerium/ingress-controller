// Package gateway contains logic for converting Gateway API configuration into Pomerium
// configuration.
package gateway

import (
	"context"
	"net/http"
	"net/url"

	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/pomerium/config"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// TranslateRoutes converts from Gateway-defined routes to Pomerium route configuration protos.
func TranslateRoutes(
	ctx context.Context,
	gatewayConfig *model.GatewayConfig,
	routeConfig *model.GatewayHTTPRouteConfig,
) []*pb.Route {
	// A single HTTPRoute may need to be represented using many Pomerium routes:
	//  - An HTTPRoute may have multiple hostnames.
	//  - An HTTPRoute may have multiple HTTPRouteRules.
	//  - An HTTPRouteRule may have multiple HTTPRouteMatches.
	// First we'll expand all HTTPRouteRules into "template" Pomerium routes, and then we'll
	// repeat each "template" route once per hostname.
	trs := templateRoutes(ctx, gatewayConfig, routeConfig)

	prs := make([]*pb.Route, 0, len(routeConfig.Hostnames)*len(trs))
	for _, h := range routeConfig.Hostnames {
		from := (&url.URL{
			Scheme: "https",
			Host:   string(h),
		}).String()
		for _, tr := range trs {
			r := proto.Clone(tr).(*pb.Route)
			r.From = from

			// Skip any routes that fail to validate.
			coreRoute, err := config.NewPolicyFromProto(r)
			if err != nil || coreRoute.Validate() != nil {
				continue
			}

			prs = append(prs, r)
		}
	}

	return prs
}

// templateRoutes converts an HTTPRoute into zero or more Pomerium routes, ignoring hostname.
func templateRoutes(
	ctx context.Context,
	gatewayConfig *model.GatewayConfig,
	routeConfig *model.GatewayHTTPRouteConfig,
) []*pb.Route {
	logger := log.FromContext(ctx)

	var prs []*pb.Route

	rules := routeConfig.Spec.Rules
	for i := range rules {
		rule := &rules[i]
		pr := &pb.Route{}

		// From the spec (near https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPRoute):
		// "Implementations MUST ignore any port value specified in the HTTP Host header while
		// performing a match and (absent of any applicable header modification configuration) MUST
		// forward this header unmodified to the backend."
		pr.PreserveHostHeader = true

		if err := applyFilters(pr, gatewayConfig, routeConfig, rule.Filters); err != nil {
			logger.Error(err, "couldn't apply filter")
			pr.Response = &pb.RouteDirectResponse{
				Status: http.StatusInternalServerError,
				Body:   "invalid filter",
			}
		} else {
			applyBackendRefs(pr, routeConfig, rule.BackendRefs)
		}

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

	return prs
}

package gateway

import (
	"fmt"
	"log"
	"net/http"

	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/pomerium/ingress-controller/model"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// applyBackendRefs translates backendRefs to a weighted set of Pomerium "To" URLs.
// [applyFilters] must be called prior to this method.
func applyBackendRefs(
	route *pb.Route,
	gc *model.GatewayHTTPRouteConfig,
	backendRefs []gateway_v1.HTTPBackendRef,
) {
	for i := range backendRefs {
		if !gc.ValidBackendRefs.Valid(gc.HTTPRoute, &backendRefs[i].BackendRef) {
			log.Printf("backendRef %v not valid", &backendRefs[i].BackendRef) // XXX
			continue
		}
		if u := backendRefToToURL(&backendRefs[i], gc.Namespace); u != "" {
			route.To = append(route.To, u)
		}
	}

	// From the spec: "If all entries in BackendRefs are invalid, and there are also no filters
	// specified in this route rule, all traffic which matches this rule MUST receive a 500 status
	// code."
	if route.Redirect == nil && len(route.To) == 0 {
		route.Response = &pb.RouteDirectResponse{
			Status: http.StatusInternalServerError,
			Body:   "no valid backend",
		}
	}
}

func backendRefToToURL(br *gateway_v1.HTTPBackendRef, defaultNamespace string) string {
	// XXX: this assumes the kind is "Service"
	namespace := defaultNamespace
	if br.Namespace != nil {
		namespace = string(*br.Namespace)
	}
	u := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", br.Name, namespace, *br.Port)

	if br.Weight != nil {
		w := *br.Weight
		// No traffic should be sent to a backend with weight equal to zero.
		if w == 0 {
			return ""
		}
		u = fmt.Sprintf("%s,%d", u, w)
	}

	return u
}

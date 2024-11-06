package gateway

import (
	"fmt"
	"net/http"

	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// applyBackendRefs translates backendRefs to a weighted set of Pomerium "To" URLs.
// [applyFilters] must be called prior to this method.
func applyBackendRefs(route *pb.Route, backendRefs []gateway_v1.HTTPBackendRef, defaultNamespace string) {
	for i := range backendRefs {
		// XXX: filter out invalid backend refs
		if u := backendRefToToURL(&backendRefs[i], defaultNamespace); u != "" {
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

package gateway

import (
	"fmt"

	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// XXX: make sure we reject any routes with filters defined on a backendRef

func applyBackendRefs(route *pb.Route, backendRefs []gateway_v1.HTTPBackendRef, defaultNamespace string) {
	for i := range backendRefs {
		if u := backendRefToToURL(&backendRefs[i], defaultNamespace); u != "" {
			route.To = append(route.To, u)
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

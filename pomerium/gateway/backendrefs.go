package gateway

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	// From the spec: "BackendRefs defines API objects where matching requests should be sent. If
	// unspecified, the rule performs no forwarding. If unspecified and no filters are specified
	// that would result in a response being sent, a 404 error code is returned."
	if route.Redirect == nil && len(backendRefs) == 0 {
		route.Response = &pb.RouteDirectResponse{
			Status: http.StatusNotFound,
			Body:   "no backend specified",
		}
		return
	}

	for i := range backendRefs {
		if !gc.ValidBackendRefs.Valid(gc.HTTPRoute, &backendRefs[i].BackendRef) {
			continue
		}
		if u, w := backendRefToToURLAndWeight(gc, &backendRefs[i]); w > 0 {
			route.To = append(route.To, u)
			route.LoadBalancingWeights = append(route.LoadBalancingWeights, w)
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

func backendRefToToURLAndWeight(
	gc *model.GatewayHTTPRouteConfig,
	br *gateway_v1.HTTPBackendRef,
) (string, uint32) {
	// Note: currently the only supported backendRef kind is "Service".
	namespace := gc.Namespace
	if br.Namespace != nil {
		namespace = string(*br.Namespace)
	}

	port := int32(*br.Port)

	// For a headless service we need the targetPort instead.
	// For now this supports only port numbers, not named ports, but this is enough to pass the
	// HTTPRouteServiceTypes conformance test cases.
	svc := gc.Services[types.NamespacedName{Namespace: namespace, Name: string(br.Name)}]
	if svc != nil && svc.Spec.ClusterIP == "None" {
		for i := range svc.Spec.Ports {
			p := &svc.Spec.Ports[i]
			if p.Port == port && p.TargetPort.Type == intstr.Int {
				port = p.TargetPort.IntVal
				break
			}
		}
	}

	u := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", br.Name, namespace, port)

	weight := uint32(1)
	if br.Weight != nil {
		weight = uint32(*br.Weight)
	}

	return u, weight
}

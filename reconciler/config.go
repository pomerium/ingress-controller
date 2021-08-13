package reconciler

import (
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/controllers"
)

func upsertRecords(cfg *pb.Config, ing *networkingv1.Ingress, tlsSecrets []*controllers.TLSSecret, sm serviceMap) error {
	ingRoutes, err := ingressToRoute(ing, sm)
	if err != nil {
		return fmt.Errorf("parsing ingress: %w", err)
	}
	ingRoutes = append(ingRoutes, debugRoute())

	ingRouteMap, err := ingRoutes.toMap()
	if err != nil {
		return fmt.Errorf("indexing new routes: %w", err)
	}
	routeMap, err := routeList(cfg.Routes).toMap()
	if err != nil {
		return fmt.Errorf("indexing current config routes: %w", err)
	}
	routeMap.merge(ingRouteMap)
	cfg.Routes = ingRouteMap.toList()
	return nil
}

func deleteRecords(cfg *pb.Config, namespacedName types.NamespacedName) error {
	return nil
}

func debugRoute() *pb.Route {
	return &pb.Route{
		Name:                      "envoy-admin",
		Id:                        "envoy-admin",
		From:                      "https://envoy-admin.localhost.pomerium.io",
		To:                        []string{"http://localhost:9901/"},
		AllowAnyAuthenticatedUser: true,
	}
}

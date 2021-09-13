package pomerium

import (
	"context"
	"fmt"
	"net/url"

	"k8s.io/apimachinery/pkg/types"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

func upsertRoutes(ctx context.Context, cfg *pb.Config, ic *model.IngressConfig) error {
	ingRoutes, err := ingressToRoutes(ctx, ic)
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
	routeMap.removeName(types.NamespacedName{Name: ic.Ingress.Name, Namespace: ic.Ingress.Namespace})
	routeMap.merge(ingRouteMap)
	cfg.Routes = routeMap.toList()
	return nil
}

func deleteRoutes(ctx context.Context, cfg *pb.Config, namespacedName types.NamespacedName) error {
	rm, err := routeList(cfg.Routes).toMap()
	if err != nil {
		return err
	}
	rm.removeName(namespacedName)
	cfg.Routes = rm.toList()
	return nil
}

func debugRoute() *pb.Route {
	r := &pb.Route{
		From:                      "https://envoy.localhost.pomerium.io",
		To:                        []string{"http://localhost:9901/"},
		Prefix:                    "/",
		AllowAnyAuthenticatedUser: true,
	}
	_ = setRouteNameID(r, types.NamespacedName{Name: "admin", Namespace: "internal-envoy"}, url.URL{Host: "envoy.localhost.pomerium.io", Path: "/"})
	return r
}

package reconciler

import (
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/controllers"
)

func routeMap(routes []*pomerium.Route) (map[string]*pomerium.Route, error) {
	m := make(map[string]*pomerium.Route)
	for _, r := range routes {
		if _, exists := m[r.Id]; exists {
			return nil, fmt.Errorf("duplicate route id=%q", r.Id)
		}
		m[r.Id] = r
	}
	return m, nil
}

func routeList(routeMap map[string]*pomerium.Route) []*pomerium.Route {
	routes := make([]*pomerium.Route, 0, len(routeMap))
	for _, r := range routeMap {
		routes = append(routes, r)
	}
	return routes
}

func upsertRecords(cfg *pomerium.Config, ing *networkingv1.Ingress, tlsSecrets []*controllers.TLSSecret) error {
	ingRoutes, err := ingressToRoute(ing)
	if err != nil {
		return fmt.Errorf("parsing ingress: %w", err)
	}
	ingRouteMap, err := routeMap(ingRoutes)
	if err != nil {
		return fmt.Errorf("indexing new routes: %w", err)
	}
	routeMap, err := routeMap(cfg.Routes)
	if err != nil {
		return fmt.Errorf("indexing current config routes: %w", err)
	}
	for id, r := range ingRouteMap {
		routeMap[id] = r
	}
	cfg.Routes = routeList(routeMap)
	return nil
}

func deleteRecords(cfg *pomerium.Config, namespacedName types.NamespacedName) error {
	return nil
}

package pomerium

import (
	"context"
	"fmt"
	"net/url"

	"k8s.io/apimachinery/pkg/types"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

func upsert(ctx context.Context, cfg *pb.Config, ic *model.IngressConfig) error {
	if err := upsertRoutes(ctx, cfg, ic); err != nil {
		return fmt.Errorf("deleting pomerium config records: %w", err)
	}

	if err := upsertCerts(cfg, ic); err != nil {
		return fmt.Errorf("updating certs: %w", err)
	}

	return nil
}

func mergeRoutes(dst *pb.Config, src routeList, name types.NamespacedName) error {
	srcMap, err := src.toMap()
	if err != nil {
		return fmt.Errorf("indexing new routes: %w", err)
	}
	dstMap, err := routeList(dst.Routes).toMap()
	if err != nil {
		return fmt.Errorf("indexing current config routes: %w", err)
	}
	// remove any existing routes of the ingress we are merging
	dstMap.removeName(name)
	dstMap.merge(srcMap)
	dst.Routes = dstMap.toList()
	return nil
}

func upsertRoutes(ctx context.Context, cfg *pb.Config, ic *model.IngressConfig) error {
	ingRoutes, err := ingressToRoutes(ctx, ic)
	if err != nil {
		return fmt.Errorf("parsing ingress: %w", err)
	}
	ingRoutes = append(ingRoutes, debugRoute())
	return mergeRoutes(cfg, ingRoutes, types.NamespacedName{Name: ic.Ingress.Name, Namespace: ic.Ingress.Namespace})
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
		From:                             "https://envoy.localhost.pomerium.io",
		To:                               []string{"http://localhost:9901/"},
		Prefix:                           "/",
		AllowPublicUnauthenticatedAccess: true,
	}
	_ = setRouteNameID(r, types.NamespacedName{Name: "admin", Namespace: "internal-envoy"}, url.URL{Host: "envoy.localhost.pomerium.io", Path: "/"})
	return r
}

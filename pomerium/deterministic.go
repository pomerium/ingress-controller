package pomerium

import (
	"cmp"
	"slices"
	"sort"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func ensureDeterministicConfigOrder(cfg *pb.Config) {
	if cfg == nil {
		return
	}
	// https://kubernetes.io/docs/concepts/services-networking/ingress/#multiple-matches
	// envoy matches according to the order routes are present in the configuration
	sort.Sort(routeList(cfg.Routes))

	if len(cfg.GetSettings().GetCertificates()) > 0 {
		slices.SortFunc(cfg.Settings.Certificates, func(a, b *pb.Settings_Certificate) int {
			return cmp.Compare(a.Id, b.Id)
		})
	}
}

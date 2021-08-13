package reconciler

import (
	"fmt"
	"sort"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
)

type routeID struct {
	Name      string
	Namespace string
	Path      string
}

type routeList []*pb.Route
type routeMap map[string]*pomerium.Route

func (routes routeList) Len() int           { return len(routes) }
func (routes routeList) Swap(i, j int)      { routes[i], routes[j] = routes[j], routes[i] }
func (routes routeList) Less(i, j int) bool { return len(routes[i].Path) < len(routes[j].Path) }
func (routes routeList) Sort()              { sort.Sort(routes) }

func (routes routeList) toMap() (routeMap, error) {
	m := make(routeMap, len(routes))
	for _, r := range routes {
		if _, exists := m[r.Name]; exists {
			return nil, fmt.Errorf("duplicate route id=%q", r.Id)
		}
		m[r.Id] = r
	}
	return m, nil
}

func (rm routeMap) toList() routeList {
	routes := make([]*pomerium.Route, 0, len(rm))
	for _, r := range rm {
		routes = append(routes, r)
	}
	return routeList(routes)
}

func (rm routeMap) merge(src routeMap) {
	for id, r := range src {
		rm[id] = r
	}
}

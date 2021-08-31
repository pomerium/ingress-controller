package pomerium

import (
	"encoding/json"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/types"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

type routeID struct {
	Name      string `json:"n"`
	Namespace string `json:"ns"`
	Path      string `json:"p"`
}

func (r *routeID) Marshal() (string, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *routeID) Unmarshal(txt string) error {
	return json.Unmarshal([]byte(txt), r)
}

type routeList []*pb.Route
type routeMap map[routeID]*pb.Route

func (routes routeList) Sort()         { sort.Sort(routes) }
func (routes routeList) Len() int      { return len(routes) }
func (routes routeList) Swap(i, j int) { routes[i], routes[j] = routes[j], routes[i] }
func (routes routeList) Less(i, j int) bool {
	routePath := func(r *pb.Route) string {
		if r.Path != "" {
			return r.Path
		}
		return r.Prefix
	}
	return routePath(routes[i]) < routePath(routes[j])
}

func (routes routeList) toMap() (routeMap, error) {
	m := make(routeMap, len(routes))
	for _, r := range routes {
		var key routeID
		if err := key.Unmarshal(r.Id); err != nil {
			return nil, fmt.Errorf("cannot decode route id %s: %w", r.Id, err)
		}
		if _, exists := m[key]; exists {
			return nil, fmt.Errorf("duplicate route %+v", key)
		}
		m[key] = r
	}
	return m, nil
}

func (rm routeMap) removeName(name types.NamespacedName) {
	for k := range rm {
		if k.Name == name.Name && k.Namespace == name.Namespace {
			delete(rm, k)
		}
	}
}

func (rm routeMap) toList() routeList {
	routes := make([]*pb.Route, 0, len(rm))
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

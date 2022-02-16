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
	Host      string `json:"h"`
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

// Less reports whether the element with
// index i should sort before the element with index j.
// as envoy parses routes as presented, we should presents routes with longer paths first
// exact Path always takes priority over Prefix matching
func (routes routeList) Less(i, j int) bool {
	// from ASC
	iFrom, jFrom := routes[i].GetFrom(), routes[j].GetFrom()
	switch {
	case iFrom < jFrom:
		return true
	case iFrom > jFrom:
		return false
	}

	// path DESC
	iPath, jPath := routes[i].GetPath(), routes[j].GetPath()
	switch {
	case iPath < jPath:
		return false
	case iPath > jPath:
		return true
	}

	// regex DESC
	iRegex, jRegex := routes[i].GetRegex(), routes[j].GetRegex()
	switch {
	case iRegex < jRegex:
		return false
	case iRegex > jRegex:
		return true
	}

	// prefix DESC
	iPrefix, jPrefix := routes[i].GetPrefix(), routes[j].GetPrefix()
	switch {
	case iPrefix < jPrefix:
		return false
	case iPrefix > jPrefix:
		return true
	}

	// finally, by id
	iID, jID := routes[i].GetId(), routes[j].GetId()
	switch {
	case iID < jID:
		return true
	case iID > jID:
		return false
	}

	return false
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
	routes := make(routeList, 0, len(rm))
	for _, r := range rm {
		routes = append(routes, r)
	}
	sort.Sort(routes)
	return routes
}

func (rm routeMap) merge(src routeMap) {
	for id, r := range src {
		rm[id] = r
	}
}

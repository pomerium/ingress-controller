package model

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// EndpointInfo represents aggregated endpoint information from EndpointSlices.
// It provides a Subsets field compatible with the existing route generation logic
// that previously used corev1.Endpoints.
type EndpointInfo struct {
	// Subsets contains the aggregated endpoints from all EndpointSlices,
	// formatted to match the corev1.EndpointSubset structure for compatibility
	// with existing route generation code.
	Subsets []corev1.EndpointSubset
}

// AggregateEndpointSlices converts a list of EndpointSlices for a service
// into an EndpointInfo with aggregated subsets. Only endpoints with
// Ready condition set to true are included. Duplicate addresses across
// multiple slices are deduplicated.
func AggregateEndpointSlices(slices []*discoveryv1.EndpointSlice) *EndpointInfo {
	if len(slices) == 0 {
		return &EndpointInfo{}
	}

	// Group endpoints by port to create subsets.
	// Each unique (port name, port number, protocol) combination becomes a subset.
	type portKey struct {
		name     string
		port     int32
		protocol corev1.Protocol
	}

	// Track unique addresses per port to avoid duplicates when multiple slices
	// contain the same endpoint (common during rolling updates or with large services).
	type subsetData struct {
		ports     []corev1.EndpointPort
		seenAddrs map[string]bool
		addresses []corev1.EndpointAddress
	}

	subsetsByPort := make(map[portKey]*subsetData)

	for _, slice := range slices {
		if slice == nil {
			continue
		}

		// Skip slices with no ports (shouldn't happen, but be defensive)
		if len(slice.Ports) == 0 {
			continue
		}

		// Collect ready addresses from this slice
		var readyAddresses []string
		for _, endpoint := range slice.Endpoints {
			// Only include ready endpoints
			if endpoint.Conditions.Ready == nil || !*endpoint.Conditions.Ready {
				continue
			}

			// Each endpoint can have multiple addresses (e.g., IPv4 and IPv6)
			readyAddresses = append(readyAddresses, endpoint.Addresses...)
		}

		// Skip if no ready addresses
		if len(readyAddresses) == 0 {
			continue
		}

		// Add addresses to subsets for each port in this slice
		for _, port := range slice.Ports {
			if port.Port == nil {
				continue
			}

			protocol := corev1.ProtocolTCP
			if port.Protocol != nil {
				protocol = *port.Protocol
			}

			portName := ""
			if port.Name != nil {
				portName = *port.Name
			}

			key := portKey{
				name:     portName,
				port:     *port.Port,
				protocol: protocol,
			}

			data, exists := subsetsByPort[key]
			if !exists {
				data = &subsetData{
					ports: []corev1.EndpointPort{{
						Name:     portName,
						Port:     *port.Port,
						Protocol: protocol,
					}},
					seenAddrs: make(map[string]bool),
				}
				subsetsByPort[key] = data
			}

			// Add unique addresses only
			for _, addr := range readyAddresses {
				if !data.seenAddrs[addr] {
					data.seenAddrs[addr] = true
					data.addresses = append(data.addresses, corev1.EndpointAddress{IP: addr})
				}
			}
		}
	}

	// Convert map to slice of subsets
	subsets := make([]corev1.EndpointSubset, 0, len(subsetsByPort))
	for _, data := range subsetsByPort {
		subsets = append(subsets, corev1.EndpointSubset{
			Addresses: data.addresses,
			Ports:     data.ports,
		})
	}

	return &EndpointInfo{
		Subsets: subsets,
	}
}

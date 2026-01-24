package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/utils/ptr"
)

func TestAggregateEndpointSlices(t *testing.T) {
	t.Run("empty slices", func(t *testing.T) {
		result := AggregateEndpointSlices(nil)
		assert.NotNil(t, result)
		assert.Empty(t, result.Subsets)

		result = AggregateEndpointSlices([]*discoveryv1.EndpointSlice{})
		assert.NotNil(t, result)
		assert.Empty(t, result.Subsets)
	})

	t.Run("single slice with ready endpoints", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{{
			Ports: []discoveryv1.EndpointPort{{
				Name:     ptr.To("http"),
				Port:     ptr.To(int32(8080)),
				Protocol: ptr.To(corev1.ProtocolTCP),
			}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			}, {
				Addresses:  []string{"10.0.0.2"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			}},
		}}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
		assert.Len(t, result.Subsets[0].Addresses, 2)
		assert.Equal(t, "10.0.0.1", result.Subsets[0].Addresses[0].IP)
		assert.Equal(t, "10.0.0.2", result.Subsets[0].Addresses[1].IP)
		require.Len(t, result.Subsets[0].Ports, 1)
		assert.Equal(t, "http", result.Subsets[0].Ports[0].Name)
		assert.Equal(t, int32(8080), result.Subsets[0].Ports[0].Port)
	})

	t.Run("filters out not ready endpoints", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{{
			Ports: []discoveryv1.EndpointPort{{
				Name:     ptr.To("http"),
				Port:     ptr.To(int32(8080)),
				Protocol: ptr.To(corev1.ProtocolTCP),
			}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			}, {
				Addresses:  []string{"10.0.0.2"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(false)},
			}, {
				Addresses:  []string{"10.0.0.3"},
				Conditions: discoveryv1.EndpointConditions{Ready: nil}, // nil means not ready
			}},
		}}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
		require.Len(t, result.Subsets[0].Addresses, 1)
		assert.Equal(t, "10.0.0.1", result.Subsets[0].Addresses[0].IP)
	})

	t.Run("multiple slices aggregated", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{
			{
				Ports: []discoveryv1.EndpointPort{{
					Name:     ptr.To("http"),
					Port:     ptr.To(int32(8080)),
					Protocol: ptr.To(corev1.ProtocolTCP),
				}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses:  []string{"10.0.0.1"},
					Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
				}},
			},
			{
				Ports: []discoveryv1.EndpointPort{{
					Name:     ptr.To("http"),
					Port:     ptr.To(int32(8080)),
					Protocol: ptr.To(corev1.ProtocolTCP),
				}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses:  []string{"10.0.0.2"},
					Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
				}},
			},
		}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
		assert.Len(t, result.Subsets[0].Addresses, 2)
	})

	t.Run("deduplicates addresses from multiple slices", func(t *testing.T) {
		// Simulates a scenario where the same endpoint appears in multiple slices
		// (can happen during rolling updates or with large services)
		slices := []*discoveryv1.EndpointSlice{
			{
				Ports: []discoveryv1.EndpointPort{{
					Name:     ptr.To("http"),
					Port:     ptr.To(int32(8080)),
					Protocol: ptr.To(corev1.ProtocolTCP),
				}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses:  []string{"10.0.0.1", "10.0.0.2"},
					Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
				}},
			},
			{
				Ports: []discoveryv1.EndpointPort{{
					Name:     ptr.To("http"),
					Port:     ptr.To(int32(8080)),
					Protocol: ptr.To(corev1.ProtocolTCP),
				}},
				Endpoints: []discoveryv1.Endpoint{{
					// Same IP as first slice
					Addresses:  []string{"10.0.0.1", "10.0.0.3"},
					Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
				}},
			},
		}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
		// Should have 3 unique addresses, not 4
		assert.Len(t, result.Subsets[0].Addresses, 3)

		// Verify all unique IPs are present
		ips := make(map[string]bool)
		for _, addr := range result.Subsets[0].Addresses {
			ips[addr.IP] = true
		}
		assert.True(t, ips["10.0.0.1"])
		assert.True(t, ips["10.0.0.2"])
		assert.True(t, ips["10.0.0.3"])
	})

	t.Run("multiple ports", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{{
			Ports: []discoveryv1.EndpointPort{
				{
					Name:     ptr.To("http"),
					Port:     ptr.To(int32(8080)),
					Protocol: ptr.To(corev1.ProtocolTCP),
				},
				{
					Name:     ptr.To("https"),
					Port:     ptr.To(int32(8443)),
					Protocol: ptr.To(corev1.ProtocolTCP),
				},
			},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			}},
		}}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 2)

		// Find each port in subsets
		var httpSubset, httpsSubset *corev1.EndpointSubset
		for i := range result.Subsets {
			if result.Subsets[i].Ports[0].Name == "http" {
				httpSubset = &result.Subsets[i]
			} else if result.Subsets[i].Ports[0].Name == "https" {
				httpsSubset = &result.Subsets[i]
			}
		}

		require.NotNil(t, httpSubset)
		require.NotNil(t, httpsSubset)
		assert.Equal(t, int32(8080), httpSubset.Ports[0].Port)
		assert.Equal(t, int32(8443), httpsSubset.Ports[0].Port)
	})

	t.Run("handles nil slice in list", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{
			nil,
			{
				Ports: []discoveryv1.EndpointPort{{
					Name:     ptr.To("http"),
					Port:     ptr.To(int32(8080)),
					Protocol: ptr.To(corev1.ProtocolTCP),
				}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses:  []string{"10.0.0.1"},
					Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
				}},
			},
		}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
	})

	t.Run("default protocol is TCP", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{{
			Ports: []discoveryv1.EndpointPort{{
				Name:     ptr.To("http"),
				Port:     ptr.To(int32(8080)),
				Protocol: nil, // nil protocol
			}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			}},
		}}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
		assert.Equal(t, corev1.ProtocolTCP, result.Subsets[0].Ports[0].Protocol)
	})

	t.Run("handles endpoints with multiple addresses", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{{
			Ports: []discoveryv1.EndpointPort{{
				Name:     ptr.To("http"),
				Port:     ptr.To(int32(8080)),
				Protocol: ptr.To(corev1.ProtocolTCP),
			}},
			Endpoints: []discoveryv1.Endpoint{{
				// Dual-stack: both IPv4 and IPv6
				Addresses:  []string{"10.0.0.1", "fd00::1"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			}},
		}}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
		assert.Len(t, result.Subsets[0].Addresses, 2)
		assert.Equal(t, "10.0.0.1", result.Subsets[0].Addresses[0].IP)
		assert.Equal(t, "fd00::1", result.Subsets[0].Addresses[1].IP)
	})

	t.Run("no ready endpoints returns empty subsets", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{{
			Ports: []discoveryv1.EndpointPort{{
				Name:     ptr.To("http"),
				Port:     ptr.To(int32(8080)),
				Protocol: ptr.To(corev1.ProtocolTCP),
			}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(false)},
			}},
		}}

		result := AggregateEndpointSlices(slices)

		assert.Empty(t, result.Subsets)
	})

	t.Run("empty port name handled", func(t *testing.T) {
		slices := []*discoveryv1.EndpointSlice{{
			Ports: []discoveryv1.EndpointPort{{
				Name:     nil, // nil port name
				Port:     ptr.To(int32(8080)),
				Protocol: ptr.To(corev1.ProtocolTCP),
			}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			}},
		}}

		result := AggregateEndpointSlices(slices)

		require.Len(t, result.Subsets, 1)
		assert.Equal(t, "", result.Subsets[0].Ports[0].Name)
	})
}

package gateway_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	icgv1alpha1 "github.com/pomerium/ingress-controller/apis/gateway/v1alpha1"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium/gateway"
)

func TestTranslate(t *testing.T) {
	t.Parallel()

	t.Run("fills source ppl", func(t *testing.T) {
		t.Parallel()

		var policy icgv1alpha1.PolicyFilter
		require.NoError(t, json.Unmarshal([]byte(`{
			"metadata": {
				"name": "example"
			},
			"spec": {
				"ppl": "allow:\n  and:\n    - email:\n        is: user@example.com"
			}
		}`), &policy))
		policyFilter, err := gateway.NewPolicyFilter(&policy)
		require.NoError(t, err)

		var route v1.HTTPRoute
		require.NoError(t, json.Unmarshal([]byte(`{
			"spec": {
				"hostnames": ["example.com"],
				"rules": [{
					"matches": [{
						"path": {
							"value": "/"
						}
					}],
					"filters": [{
						"type": "ExtensionRef",
						"extensionRef": {
							"group": "gateway.pomerium.io",
							"kind": "PolicyFilter",
							"name": "example"
						}
					}]
				}]
			}
		}`), &route))

		result := gateway.TranslateRoutes(t.Context(),
			&model.GatewayConfig{
				ExtensionFilters: map[model.ExtensionFilterKey]model.ExtensionFilter{
					{Kind: "PolicyFilter", Namespace: "", Name: "example"}: policyFilter,
				},
			},
			&model.GatewayHTTPRouteConfig{
				HTTPRoute: &route,
				Hostnames: []v1.Hostname{"example.com"},
			})
		if assert.Len(t, result, 1) {
			if assert.Len(t, result[0].Policies, 1) {
				assert.Equal(t, policy.Spec.PPL, result[0].Policies[0].GetSourcePpl(),
					"should fill source ppl")
			}
		}
	})
}

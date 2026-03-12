package pomerium

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pomerium/ingress-controller/model"
)

func TestIngressToPolicy(t *testing.T) {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress",
			Namespace: "my-namespace",
			Annotations: map[string]string{
				"a/allowed_users":      `["user-id-1", "user-email-2@example.com"]`,
				"a/allowed_domains":    `["a.example.com", "b.example.com"]`,
				"a/allowed_idp_claims": `{ "foo": ["bar", "baz"] }`,
				"a/policy": `deny:
  or:
    - source_ip: 1.2.3.4`,
			},
		},
	}
	ic := &model.IngressConfig{
		AnnotationPrefix: "a",
		Ingress:          ingress,
	}
	p, err := ingressToPolicy(ic)
	require.NoError(t, err)
	require.NotNil(t, p.Name)
	assert.Equal(t, "my-namespace-my-ingress-policy", *p.Name)
	require.NotNil(t, p.SourcePpl)
	assert.JSONEq(t, `[
  {
    "allow": {
      "or": [
        {
          "domain": {
            "is": "a.example.com"
          }
        },
        {
          "domain": {
            "is": "b.example.com"
          }
        },
        {
          "claim/foo": "bar"
        },
        {
          "claim/foo": "baz"
        },
        {
          "user": {
            "is": "user-id-1"
          }
        },
        {
          "email": {
            "is": "user-id-1"
          }
        },
        {
          "user": {
            "is": "user-email-2@example.com"
          }
        },
        {
          "email": {
            "is": "user-email-2@example.com"
          }
        }
      ]
    }
  },
  {
    "deny": {
      "or": [
        {
          "source_ip": "1.2.3.4"
        }
      ]
    }
  }
]`, *p.SourcePpl)
}

func TestIngressToPolicy_Empty(t *testing.T) {
	// ingressToPolicy should return nil when called with an Ingress with no
	// Pomerium policy annotations.
	ingress := &networkingv1.Ingress{}
	ic := &model.IngressConfig{
		AnnotationPrefix: "a",
		Ingress:          ingress,
	}
	p, err := ingressToPolicy(ic)
	require.NoError(t, err)
	assert.Nil(t, p)
}

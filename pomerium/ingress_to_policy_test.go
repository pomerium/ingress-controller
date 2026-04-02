package pomerium

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeysToPolicy(t *testing.T) {
	kv, err := removeKeyPrefix(map[string]string{
		"a/allowed_users":      `["user-id-1", "user-email-2@example.com"]`,
		"a/allowed_domains":    `["a.example.com", "b.example.com"]`,
		"a/allowed_idp_claims": `{ "foo": ["bar", "baz"] }`,
		"a/policy": `deny:
  or:
    - source_ip: 1.2.3.4`,
	}, "a")
	require.NoError(t, err)

	p, err := keysToPolicy(kv, "POLICY-NAME")
	require.NoError(t, err)
	require.NotNil(t, p.Name)
	assert.Equal(t, "POLICY-NAME", *p.Name)
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

func TestKeysToPolicy_Empty(t *testing.T) {
	// keysToPolicy should return nil when there are no policy-related annotations.
	kv, err := removeKeyPrefix(map[string]string{
		"a/tls_skip_verify": "true",
	}, "a")
	require.NoError(t, err)

	p, err := keysToPolicy(kv, "POLICY-NAME")
	require.NoError(t, err)
	assert.Nil(t, p)
}

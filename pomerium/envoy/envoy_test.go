package envoy

import (
	"context"
	"os"
	"testing"
	"time"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletedBinary(t *testing.T) {
	p1, err := setup()
	assert.NoError(t, err)

	err = os.Remove(p1)
	assert.NoError(t, err)

	p2, err := setup()
	assert.NoError(t, err)

	assert.NotEqual(t, p1, p2)
}

func TestValidate(t *testing.T) {
	ctx, clearTimeout := context.WithTimeout(context.Background(), time.Second*10)
	defer clearTimeout()

	t.Run("valid", func(t *testing.T) {
		res, err := Validate(ctx, &envoy_config_bootstrap_v3.Bootstrap{}, uuid.NewString())
		require.NoError(t, err)
		assert.True(t, res.Valid)
		assert.Equal(t, "OK", res.Message)
	})
	t.Run("invalid", func(t *testing.T) {
		res, err := Validate(ctx, &envoy_config_bootstrap_v3.Bootstrap{
			Admin: &envoy_config_bootstrap_v3.Admin{
				Address: &envoy_config_core_v3.Address{
					Address: &envoy_config_core_v3.Address_SocketAddress{
						SocketAddress: &envoy_config_core_v3.SocketAddress{
							Protocol: envoy_config_core_v3.SocketAddress_TCP,
							Address:  "<<INVALID>>",
							PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
								PortValue: 1234,
							},
						},
					},
				},
			},
		}, uuid.NewString())
		require.NoError(t, err)
		assert.False(t, res.Valid)
		assert.Contains(t, res.Message, "malformed IP address: <<INVALID>>")
	})
}

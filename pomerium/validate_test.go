package pomerium

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// TestValidateRoutesOnlyJWTRoute reproduces the databroker-mode preflight: the
// routes record is validated on its own, without the settings record that
// carries identity_providers. A JWT bearer route must not be rejected just
// because its providers live in a different record.
func TestValidateRoutesOnlyJWTRoute(t *testing.T) {
	cfg := &pb.Config{
		Routes: []*pb.Route{{
			Name:              proto.String("jwt-route"),
			From:              "https://jwt.localhost.pomerium.io",
			To:                []string{"http://upstream.default.svc.cluster.local"},
			BearerTokenFormat: pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_JWT.Enum(),
			IdentityProviders: []string{"cluster"},
		}},
	}
	require.NoError(t, validate(context.Background(), cfg, "test"))
}

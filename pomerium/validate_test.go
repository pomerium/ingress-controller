package pomerium

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

// The databroker preflight validates the routes record without the settings
// record that holds identity_providers; a JWT route must still pass.
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

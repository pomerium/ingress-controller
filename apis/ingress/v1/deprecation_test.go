package v1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

func TestDeprecations(t *testing.T) {
	msgs, err := api.GetDeprecations(&api.PomeriumSpec{
		Authenticate: new(api.Authenticate),
		IdentityProvider: &api.IdentityProvider{
			Provider: "google", URL: proto.String("http://google.com"),
			ServiceAccountFromSecret: proto.String("secret"),
			RefreshDirectory: &api.RefreshDirectorySettings{
				Interval: v1.Duration{Duration: time.Minute},
				Timeout:  v1.Duration{Duration: time.Minute},
			},
		},
		Certificates: []string{},
		Secrets:      "",
	})
	require.NoError(t, err)
	require.Len(t, msgs, 2)
}

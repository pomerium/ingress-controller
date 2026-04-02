package pomerium

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	"github.com/pomerium/sdk-go/proto/pomerium"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/model"
	corev1 "k8s.io/api/core/v1"
)

func TestAPIReconciler_SetConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	k8sClient := controllers_mock.NewMockClient(ctrl)
	apiClient := controllers_mock.NewMockSDKClient(ctrl)

	r := APIReconciler{
		apiClient:  apiClient,
		k8sClient:  k8sClient,
		secretsMap: model.NewTLSSecretsMap(),
	}

	cfg := &model.Config{
		Pomerium: icsv1.Pomerium{
			Spec: icsv1.PomeriumSpec{
				Authenticate: &icsv1.Authenticate{
					URL: "https://authenticate.localhost.pomerium.io",
				},
				IdentityProvider: &icsv1.IdentityProvider{
					Provider: "oidc",
					URL:      proto.String("https://idp.example.com"),
					Secret:   "test/idp-client-secret",
				},
				PassIdentityHeaders: proto.Bool(true),
			},
		},
		IdpSecret: &corev1.Secret{
			Data: map[string][]byte{
				"client_id":     []byte("CLIENT_ID"),
				"client_secret": []byte("CLIENT_SECRET"),
			},
		},
	}

	t.Run("settings changed", func(t *testing.T) {
		// APIReconciler should first call GetSettings() to determine if it needs to sync any changes...
		apiClient.EXPECT().GetSettings(gomock.Any(), connect.NewRequest(&pomerium.GetSettingsRequest{})).
			Return(&connect.Response[pomerium.GetSettingsResponse]{
				Msg: &pomerium.GetSettingsResponse{
					Settings: &pomerium.Settings{
						Id: proto.String("settings-id-123"),
					},
				},
			}, nil)

		// ...and then call UpdateSettings() once it knows there are changes to sync.
		apiClient.EXPECT().UpdateSettings(gomock.Any(), RequestEq(&pomerium.UpdateSettingsRequest{
			Settings: &pomerium.Settings{
				AuthenticateServiceUrl: proto.String("https://authenticate.localhost.pomerium.io"),
				IdpClientId:            proto.String("CLIENT_ID"),
				IdpClientSecret:        proto.String("CLIENT_SECRET"),
				IdpProvider:            proto.String("oidc"),
				IdpProviderUrl:         proto.String("https://idp.example.com"),
				PassIdentityHeaders:    proto.Bool(true),
			},
		})).Return(&connect.Response[pomerium.UpdateSettingsResponse]{
			Msg: &pomerium.UpdateSettingsResponse{},
		}, nil)

		changes, err := r.SetConfig(t.Context(), cfg)
		assert.True(t, changes)
		assert.NoError(t, err)
	})

	t.Run("settings unchanged", func(t *testing.T) {
		// If the settings already match, there should be no UpdateSettings() call.
		apiClient.EXPECT().GetSettings(gomock.Any(), connect.NewRequest(&pomerium.GetSettingsRequest{})).
			Return(&connect.Response[pomerium.GetSettingsResponse]{
				Msg: &pomerium.GetSettingsResponse{
					Settings: &pomerium.Settings{
						Id: proto.String("settings-id-123"),

						AuthenticateServiceUrl: proto.String("https://authenticate.localhost.pomerium.io"),
						IdpClientId:            proto.String("CLIENT_ID"),
						IdpClientSecret:        proto.String("CLIENT_SECRET"),
						IdpProvider:            proto.String("oidc"),
						IdpProviderUrl:         proto.String("https://idp.example.com"),
						PassIdentityHeaders:    proto.Bool(true)},
				},
			}, nil)

		changes, err := r.SetConfig(t.Context(), cfg)
		assert.False(t, changes)
		assert.NoError(t, err)
	})
}

type requestMatcher[T any, P interface {
	proto.Message
	*T
}] struct {
	expected P
}

func RequestEq[T any, P interface {
	proto.Message
	*T
}](expected P) gomock.Matcher {
	return requestMatcher[T, P]{expected: expected}
}

func (m requestMatcher[T, P]) Matches(x interface{}) bool {
	req, ok := x.(*connect.Request[T])
	if !ok {
		return false
	}
	return proto.Equal(m.expected, P(req.Msg))
}

func (m requestMatcher[T, P]) String() string {
	return fmt.Sprintf("request with msg %[1]v (%[1]T)", m.expected)
}

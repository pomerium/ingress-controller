package settings_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/controllers/settings"
)

func TestFetch(t *testing.T) {
	ctx := context.Background()
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	settingsName := types.NamespacedName{Namespace: "pomerium", Name: "settings"}

	for _, name := range []string{"idp-secrets", "bootstrap-secrets"} {
		nn := types.NamespacedName{Namespace: "pomerium", Name: name}
		mc.EXPECT().Get(ctx, nn, gomock.AssignableToTypeOf(new(corev1.Secret))).
			Do(func(_ context.Context, _ types.NamespacedName, dst *corev1.Secret) {
				*dst = corev1.Secret{}
			}).AnyTimes().Return(nil)
	}

	for _, tc := range []struct {
		name        string
		spec        icsv1.SettingsSpec
		expectError bool
	}{
		{"ok", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
		}, false},
		{"bootstrap secret is mandatory", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
		}, true},
		{"idp secret is mandatory", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mc.EXPECT().Get(ctx, settingsName, gomock.AssignableToTypeOf(new(icsv1.Settings))).
				Do(func(_ context.Context, _ types.NamespacedName, dst *icsv1.Settings) {
					*dst = icsv1.Settings{
						ObjectMeta: metav1.ObjectMeta{
							Name:      settingsName.Name,
							Namespace: settingsName.Namespace,
						},
						Spec: tc.spec,
					}
				}).
				Return(nil)

			_, err := settings.FetchConfig(ctx, mc, settingsName)
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			if !assert.NoError(t, err) {
				return
			}
		})
	}
}

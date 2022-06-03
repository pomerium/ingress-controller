package settings_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/controllers/settings"
)

func TestFetch(t *testing.T) {
	ctx := context.Background()
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	name := types.NamespacedName{Namespace: "pomerium", Name: "settings"}

	for _, tc := range []struct {
		name        string
		spec        icsv1.SettingsSpec
		expectError bool
	}{
		{"secret is mandatory", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{},
			Certificates:     []string{},
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mc.EXPECT().Get(ctx, name, gomock.AssignableToTypeOf(new(icsv1.Settings))).
				Do(func(_ context.Context, _ types.NamespacedName, dst *icsv1.Settings) {
					*dst = icsv1.Settings{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name.Name,
							Namespace: name.Namespace,
						},
						Spec: tc.spec,
					}
				}).
				Return(nil)

			_, err := settings.FetchConfig(ctx, mc, name)
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

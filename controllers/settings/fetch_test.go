package settings_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/model"
)

func TestFetchConstraints(t *testing.T) {
	ctx := context.Background()
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	settingsName := types.NamespacedName{Namespace: "pomerium", Name: "settings"}

	getSecret := func(want types.NamespacedName, data map[string][]byte, tp corev1.SecretType) func(_ context.Context, name types.NamespacedName, dst *corev1.Secret) {
		return func(_ context.Context, got types.NamespacedName, dst *corev1.Secret) {
			require.Equal(t, want, got)
			t.Log(want)
			*dst = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: got.Name, Namespace: got.Namespace},
				Data:       data,
				Type:       tp,
			}
		}
	}

	for name, data := range map[string]struct {
		corev1.SecretType
		data map[string][]byte
	}{
		"idp-secrets":       {},
		"bootstrap-secrets": {},
		"redis": {
			corev1.SecretTypeOpaque,
			map[string][]byte{
				model.StorageConnectionStringKey: []byte("redis://"),
			},
		},
		"redis-ca": {
			corev1.SecretTypeOpaque,
			map[string][]byte{
				model.CAKey: []byte("ca-data"),
			},
		},
		"redis-tls": {
			corev1.SecretTypeTLS,
			map[string][]byte{
				corev1.TLSCertKey:       []byte("cert-data"),
				corev1.TLSPrivateKeyKey: []byte("key-data"),
			},
		},
		"postgres": {
			corev1.SecretTypeOpaque,
			map[string][]byte{
				model.StorageConnectionStringKey: []byte("postgresql:///mydb?host=localhost&port=5433"),
			},
		},
		"postgres-ca": {
			corev1.SecretTypeOpaque,
			map[string][]byte{
				model.CAKey: []byte("ca-data"),
			},
		},
		"postgres-tls": {
			corev1.SecretTypeTLS,
			map[string][]byte{
				corev1.TLSCertKey:       []byte("cert-data"),
				corev1.TLSPrivateKeyKey: []byte("key-data"),
			},
		},
	} {
		nn := types.NamespacedName{Namespace: "pomerium", Name: name}
		mc.EXPECT().
			Get(ctx, nn, gomock.AssignableToTypeOf(new(corev1.Secret))).
			Do(getSecret(nn, data.data, data.SecretType)).
			MinTimes(1).
			Return(nil)
	}

	for _, tc := range []struct {
		name  string
		spec  icsv1.SettingsSpec
		check func(assert.TestingT, error, ...any) bool
	}{
		{"ok", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
		}, assert.NoError},
		{"bootstrap secret is mandatory", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
		}, assert.Error},
		{"idp secret is mandatory", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
		}, assert.Error},
		{"no empty storage", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{},
		}, assert.Error},
		{"redis: secret missing", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{Redis: &icsv1.RedisStorage{}},
		}, assert.Error},
		{"redis: secret present", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{Redis: &icsv1.RedisStorage{Secret: "pomerium/redis"}},
		}, assert.NoError},
		{"redis: ca + tls", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage: &icsv1.Storage{Redis: &icsv1.RedisStorage{
				Secret:    "pomerium/redis",
				CASecret:  proto.String("pomerium/redis-ca"),
				TLSSecret: proto.String("pomerium/redis-tls"),
			}},
		}, assert.NoError},
		{"postgres: secret missing", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{Postgres: &icsv1.PostgresStorage{}},
		}, assert.Error},
		{"postgres: secret present", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{Postgres: &icsv1.PostgresStorage{Secret: "pomerium/postgres"}},
		}, assert.NoError},
		{"postgres: ca + tls", icsv1.SettingsSpec{
			Authenticate:     icsv1.Authenticate{},
			IdentityProvider: icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage: &icsv1.Storage{Postgres: &icsv1.PostgresStorage{
				Secret:    "pomerium/postgres",
				CASecret:  proto.String("pomerium/postgres-ca"),
				TLSSecret: proto.String("pomerium/postgres-tls"),
			}},
		}, assert.NoError},
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
			tc.check(t, err)
		})
	}
}

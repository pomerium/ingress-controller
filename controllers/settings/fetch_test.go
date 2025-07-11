package settings_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/controllers/settings"
	"github.com/pomerium/ingress-controller/model"
)

func TestFetchConstraints(t *testing.T) {
	ctx := context.Background()
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	settingsName := types.NamespacedName{Namespace: "pomerium", Name: "settings"}

	getSecret := func(want types.NamespacedName, data map[string][]byte, tp corev1.SecretType) func(_ context.Context, name types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) {
		return func(_ context.Context, got types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) {
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
		"postgres": {
			corev1.SecretTypeOpaque,
			map[string][]byte{
				//cspell:disable-next-line
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
		spec  icsv1.PomeriumSpec
		check func(assert.TestingT, error, ...any) bool
	}{
		{"ok", icsv1.PomeriumSpec{
			Authenticate:     new(icsv1.Authenticate),
			IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
		}, assert.NoError},
		{"bootstrap secret is mandatory", icsv1.PomeriumSpec{
			Authenticate:     new(icsv1.Authenticate),
			IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
		}, assert.Error},
		{"idp secret is mandatory", icsv1.PomeriumSpec{
			Authenticate:     new(icsv1.Authenticate),
			IdentityProvider: &icsv1.IdentityProvider{},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
		}, assert.Error},
		{"no empty storage", icsv1.PomeriumSpec{
			Authenticate:     new(icsv1.Authenticate),
			IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{},
		}, assert.Error},
		{"postgres: secret missing", icsv1.PomeriumSpec{
			Authenticate:     new(icsv1.Authenticate),
			IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{Postgres: &icsv1.PostgresStorage{}},
		}, assert.Error},
		{"postgres: secret present", icsv1.PomeriumSpec{
			Authenticate:     new(icsv1.Authenticate),
			IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
			Certificates:     []string{},
			Secrets:          "pomerium/bootstrap-secrets",
			Storage:          &icsv1.Storage{Postgres: &icsv1.PostgresStorage{Secret: "pomerium/postgres"}},
		}, assert.NoError},
		{"postgres: ca + tls", icsv1.PomeriumSpec{
			Authenticate:     new(icsv1.Authenticate),
			IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
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
			mc.EXPECT().Get(ctx, settingsName,
				gomock.AssignableToTypeOf(new(icsv1.Pomerium)),
			).Do(func(_ context.Context, _ types.NamespacedName, dst *icsv1.Pomerium, _ ...client.GetOptions) {
				*dst = icsv1.Pomerium{
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

func TestFetchConfigCertsMissingSecrets(t *testing.T) {
	ctx := context.Background()
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	settingsName := types.NamespacedName{Namespace: "pomerium", Name: "settings"}

	// This is important for cert-manager HTTP-01 challenge integration
	// Related: https://github.com/pomerium/ingress-controller/issues/683
	t.Run("missing certificate secrets should not fail", func(t *testing.T) {
		mc.EXPECT().Get(ctx, settingsName,
			gomock.AssignableToTypeOf(new(icsv1.Pomerium)),
		).Do(func(_ context.Context, _ types.NamespacedName, dst *icsv1.Pomerium, _ ...client.GetOptions) {
			*dst = icsv1.Pomerium{
				ObjectMeta: metav1.ObjectMeta{
					Name:      settingsName.Name,
					Namespace: settingsName.Namespace,
				},
				Spec: icsv1.PomeriumSpec{
					Authenticate:     new(icsv1.Authenticate),
					IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
					Certificates:     []string{"pomerium/missing-cert-1", "pomerium/missing-cert-2"},
					Secrets:          "pomerium/bootstrap-secrets",
				},
			}
		}).Return(nil)

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "bootstrap-secrets"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Do(func(_ context.Context, name types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) {
			*dst = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Data:       map[string][]byte{},
				Type:       corev1.SecretTypeOpaque,
			}
		}).Return(nil)

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "idp-secrets"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Do(func(_ context.Context, name types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) {
			*dst = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Data:       map[string][]byte{},
				Type:       corev1.SecretTypeOpaque,
			}
		}).Return(nil)

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "missing-cert-1"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Return(errors.NewNotFound(schema.GroupResource{Group: "", Resource: "secrets"}, "missing-cert-1"))

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "missing-cert-2"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Return(errors.NewNotFound(schema.GroupResource{Group: "", Resource: "secrets"}, "missing-cert-2"))

		cfg, err := settings.FetchConfig(ctx, mc, settingsName)
		require.NoError(t, err, "FetchConfig should not fail when certificate secrets are missing")
		assert.NotNil(t, cfg)

		assert.Len(t, cfg.Certs, 0, "No certificates should be loaded when secrets are missing")

		assert.NotNil(t, cfg.Secrets, "Bootstrap secrets should be loaded")
		assert.NotNil(t, cfg.IdpSecret, "IDP secrets should be loaded")
	})

	t.Run("partial certificate secrets should load existing ones", func(t *testing.T) {
		mc.EXPECT().Get(ctx, settingsName,
			gomock.AssignableToTypeOf(new(icsv1.Pomerium)),
		).Do(func(_ context.Context, _ types.NamespacedName, dst *icsv1.Pomerium, _ ...client.GetOptions) {
			*dst = icsv1.Pomerium{
				ObjectMeta: metav1.ObjectMeta{
					Name:      settingsName.Name,
					Namespace: settingsName.Namespace,
				},
				Spec: icsv1.PomeriumSpec{
					Authenticate:     new(icsv1.Authenticate),
					IdentityProvider: &icsv1.IdentityProvider{Secret: "pomerium/idp-secrets"},
					Certificates:     []string{"pomerium/existing-cert", "pomerium/missing-cert"},
					Secrets:          "pomerium/bootstrap-secrets",
				},
			}
		}).Return(nil)

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "bootstrap-secrets"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Do(func(_ context.Context, name types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) {
			*dst = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Data:       map[string][]byte{},
				Type:       corev1.SecretTypeOpaque,
			}
		}).Return(nil)

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "idp-secrets"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Do(func(_ context.Context, name types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) {
			*dst = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Data:       map[string][]byte{},
				Type:       corev1.SecretTypeOpaque,
			}
		}).Return(nil)

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "existing-cert"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Do(func(_ context.Context, name types.NamespacedName, dst *corev1.Secret, _ ...client.GetOption) {
			*dst = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Data: map[string][]byte{
					corev1.TLSCertKey:       []byte("cert-data"),
					corev1.TLSPrivateKeyKey: []byte("key-data"),
				},
				Type: corev1.SecretTypeTLS,
			}
		}).Return(nil)

		mc.EXPECT().Get(ctx, types.NamespacedName{Namespace: "pomerium", Name: "missing-cert"},
			gomock.AssignableToTypeOf(new(corev1.Secret)),
		).Return(errors.NewNotFound(schema.GroupResource{Group: "", Resource: "secrets"}, "missing-cert"))

		cfg, err := settings.FetchConfig(ctx, mc, settingsName)
		require.NoError(t, err, "FetchConfig should not fail with partial certificate secrets")
		assert.NotNil(t, cfg)

		assert.Len(t, cfg.Certs, 1, "Only existing certificate should be loaded")
		existingCertKey := types.NamespacedName{Namespace: "pomerium", Name: "existing-cert"}
		assert.Contains(t, cfg.Certs, existingCertKey, "Existing certificate should be in the map")
		assert.Equal(t, corev1.SecretTypeTLS, cfg.Certs[existingCertKey].Type, "Certificate should be TLS type")
	})
}

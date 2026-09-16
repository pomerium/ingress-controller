package pomerium_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
	"github.com/pomerium/pomerium/config"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func TestApplyConfig(t *testing.T) {
	t.Parallel()

	t.Run("AllowUpgrades", func(t *testing.T) {
		t.Parallel()
		var dst pb.Config

		assert.NoError(t, pomerium.ApplyConfig(t.Context(), &dst, &model.Config{}))
		assert.Nil(t, dst.Settings.GetAllowUpgrades(),
			"should default to nil")

		assert.NoError(t, pomerium.ApplyConfig(t.Context(), &dst, &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					AllowUpgrades: new([]string{"a", "b", "c"}),
				},
			},
		}))
		assert.Equal(t, []string{"a", "b", "c"}, dst.Settings.GetAllowUpgrades().GetValues(),
			"should set allow upgrades")
	})
}

func TestApplyConfig_DownstreamMTLS(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		expect *pb.DownstreamMtlsSettings
		mtls   *v1.DownstreamMTLS
	}{
		{"nil", nil, nil},
		{"empty", &pb.DownstreamMtlsSettings{}, &v1.DownstreamMTLS{}},
		{
			"ca",
			&pb.DownstreamMtlsSettings{Ca: proto.String("AQIDBA==")},
			&v1.DownstreamMTLS{CA: []byte{1, 2, 3, 4}},
		},
		{
			"crl",
			&pb.DownstreamMtlsSettings{Crl: proto.String("BQYHCA==")},
			&v1.DownstreamMTLS{CRL: []byte{5, 6, 7, 8}},
		},
		{
			"policy_with_default_deny",
			&pb.DownstreamMtlsSettings{Enforcement: pb.MtlsEnforcementMode_POLICY_WITH_DEFAULT_DENY.Enum()},
			&v1.DownstreamMTLS{Enforcement: proto.String("policy_with_default_deny")},
		},
		{
			"policy",
			&pb.DownstreamMtlsSettings{Enforcement: pb.MtlsEnforcementMode_POLICY.Enum()},
			&v1.DownstreamMTLS{Enforcement: proto.String("policy")},
		},
		{
			"reject_connection",
			&pb.DownstreamMtlsSettings{Enforcement: pb.MtlsEnforcementMode_REJECT_CONNECTION.Enum()},
			&v1.DownstreamMTLS{Enforcement: proto.String("REJECT_CONNECTION")},
		},
		{
			"unknown",
			&pb.DownstreamMtlsSettings{},
			&v1.DownstreamMTLS{Enforcement: proto.String("unknown")},
		},
		{
			"dns",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_DNS, Pattern: "DNS"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{DNS: "DNS"}},
		},
		{
			"email",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_EMAIL, Pattern: "EMAIL"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{Email: "EMAIL"}},
		},
		{
			"ip address",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_IP_ADDRESS, Pattern: "IP_ADDRESS"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{IPAddress: "IP_ADDRESS"}},
		},
		{
			"uri",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_URI, Pattern: "URI"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{URI: "URI"}},
		},
		{
			"user principal name",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_USER_PRINCIPAL_NAME, Pattern: "USER_PRINCIPAL_NAME"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{UserPrincipalName: "USER_PRINCIPAL_NAME"}},
		},
		{
			"max verify depth",
			&pb.DownstreamMtlsSettings{MaxVerifyDepth: proto.Uint32(23)},
			&v1.DownstreamMTLS{MaxVerifyDepth: proto.Uint32(23)},
		},
	} {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					DownstreamMTLS: tc.mtls,
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(ctx, dst, src)
		assert.NoError(t, err,
			"should have no error in %s", tc.name)
		assert.Empty(t, cmp.Diff(tc.expect, dst.Settings.DownstreamMtls, protocmp.Transform()),
			"should match in %s", tc.name)
	}
}

func TestApplyConfig_MCPAllowedASMetadataDomains(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name    string
		domains []string
		expect  []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, []string{}},
		{"single domain", []string{"api.githubcopilot.com"}, []string{"api.githubcopilot.com"}},
		{"wildcard", []string{"*.example.com"}, []string{"*.example.com"}},
		{"multiple domains", []string{"api.githubcopilot.com", "*.github.com"}, []string{"api.githubcopilot.com", "*.github.com"}},
	} {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					MCPAllowedASMetadataDomains: tc.domains,
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(ctx, dst, src)
		assert.NoError(t, err, "should have no error in %s", tc.name)
		assert.Equal(t, tc.expect, dst.Settings.McpAllowedAsMetadataDomains,
			"should match in %s", tc.name)
	}
}

func TestApplyConfig_IdentityProvider(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		src := &model.Config{}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		assert.NoError(t, err)
		assert.Nil(t, dst.GetSettings().IdpProvider)
	})
	t.Run("hosted", func(t *testing.T) {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					IdentityProvider: &v1.IdentityProvider{
						Provider: "hosted",
					},
				},
			},
			// no IdpSecret should be required for hosted authenticate
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		assert.NoError(t, err)
		assert.Equal(t, "hosted", dst.GetSettings().GetIdpProvider())
	})
	t.Run("google", func(t *testing.T) {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					IdentityProvider: &v1.IdentityProvider{
						Provider: "google",
					},
				},
			},
			IdpSecret: &corev1.Secret{
				Data: map[string][]byte{
					"client_id":     []byte("my-client-id"),
					"client_secret": []byte("my-client-secret"),
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		assert.NoError(t, err)
		assert.Equal(t, "google", dst.GetSettings().GetIdpProvider())
		assert.Equal(t, "my-client-id", dst.GetSettings().GetIdpClientId())
		assert.Equal(t, "my-client-secret", dst.GetSettings().GetIdpClientSecret())
	})
	t.Run("missing secret", func(t *testing.T) {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					IdentityProvider: &v1.IdentityProvider{
						Provider: "google",
					},
				},
			},
			// IdpSecret is required but missing
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		assert.ErrorContains(t, err, "idp secret: is required")
	})
}

func TestApplyConfig_RequestNormalizationOptions(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		src := &model.Config{}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		require.NoError(t, err)
		assert.Nil(t, dst.GetSettings().NormalizePath)
		assert.Nil(t, dst.GetSettings().MergeSlashes)
		assert.Nil(t, dst.GetSettings().PathWithEscapedSlashesAction)
		assert.Nil(t, dst.GetSettings().HeadersWithUnderscoresAction)
	})
	t.Run("no normalization", func(t *testing.T) {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					NormalizePath:                new(false),
					MergeSlashes:                 new(false),
					PathWithEscapedSlashesAction: new("keep_unchanged"),
					HeadersWithUnderscoresAction: new("allow"),
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		require.NoError(t, err)
		assert.Equal(t, new(false), dst.GetSettings().NormalizePath)
		assert.Equal(t, new(false), dst.GetSettings().MergeSlashes)
		assert.Equal(t, pb.PathWithEscapedSlashesAction_PATH_WITH_ESCAPED_SLASHES_ACTION_KEEP_UNCHANGED.Enum(), dst.GetSettings().PathWithEscapedSlashesAction)
		assert.Equal(t, pb.HeadersWithUnderscoresAction_HEADERS_WITH_UNDERSCORES_ACTION_ALLOW.Enum(), dst.GetSettings().HeadersWithUnderscoresAction)
	})
	t.Run("other", func(t *testing.T) {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					NormalizePath:                new(true),
					MergeSlashes:                 new(true),
					PathWithEscapedSlashesAction: new("unescape_and_redirect"),
					HeadersWithUnderscoresAction: new("drop_header"),
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		require.NoError(t, err)
		assert.Equal(t, new(true), dst.GetSettings().NormalizePath)
		assert.Equal(t, new(true), dst.GetSettings().MergeSlashes)
		assert.Equal(t, pb.PathWithEscapedSlashesAction_PATH_WITH_ESCAPED_SLASHES_ACTION_UNESCAPE_AND_REDIRECT.Enum(), dst.GetSettings().PathWithEscapedSlashesAction)
		assert.Equal(t, pb.HeadersWithUnderscoresAction_HEADERS_WITH_UNDERSCORES_ACTION_DROP_HEADER.Enum(), dst.GetSettings().HeadersWithUnderscoresAction)
	})
	t.Run("unknown pathWithEscapedSlashesAction", func(t *testing.T) {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					PathWithEscapedSlashesAction: new("foobar"),
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		assert.ErrorContains(t, err, `unknown pathWithEscapedSlashesAction "foobar"`)
	})
	t.Run("unknown headersWithUnderscoresAction", func(t *testing.T) {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					HeadersWithUnderscoresAction: new("foobar"),
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(t.Context(), dst, src)
		assert.ErrorContains(t, err, `unknown headersWithUnderscoresAction "foobar"`)
	})
}

func TestApplyConfig_BearerTokenFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		format    *string
		expect    *pb.BearerTokenFormat
		expectErr string
	}{
		{"unset", nil, nil, ""},
		{"empty", new(""), pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_UNKNOWN.Enum(), ""},
		{"default", new("default"), pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_DEFAULT.Enum(), ""},
		{"idp access token", new("idp_access_token"), pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_IDP_ACCESS_TOKEN.Enum(), ""},
		{"idp identity token", new("idp_identity_token"), pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_IDP_IDENTITY_TOKEN.Enum(), ""},
		{"jwt", new("jwt"), pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_JWT.Enum(), ""},
		{"unknown", new("nonsense"), nil, "unknown bearerTokenFormat nonsense"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dst := new(pb.Config)
			err := pomerium.ApplyConfig(t.Context(), dst, &model.Config{
				Pomerium: v1.Pomerium{
					Spec: v1.PomeriumSpec{BearerTokenFormat: tc.format},
				},
			})
			if tc.expectErr != "" {
				assert.ErrorContains(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expect, dst.GetSettings().BearerTokenFormat)
		})
	}
}

func TestApplyConfig_IdentityProviders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		src    map[string]v1.JWTIdentityProvider
		expect map[string]*pb.IdentityProvider
	}{
		{"unset", nil, nil},
		{"empty", map[string]v1.JWTIdentityProvider{}, nil},
		{
			"in-cluster",
			map[string]v1.JWTIdentityProvider{
				"cluster": {Issuer: "kubernetes:///", Audiences: []string{"pomerium"}},
			},
			map[string]*pb.IdentityProvider{
				"cluster": {Issuer: "kubernetes:///", Audiences: []string{"pomerium"}},
			},
		},
		{
			"all fields, multiple providers",
			map[string]v1.JWTIdentityProvider{
				"cluster": {Issuer: "kubernetes:///", Audiences: []string{"pomerium"}},
				"github": {
					Issuer:        "https://token.actions.githubusercontent.com",
					JWKSURL:       new("https://token.actions.githubusercontent.com/.well-known/jwks"),
					SupportedAlgs: []string{"RS256", "ES256"},
					Audiences:     []string{"https://pomerium.example.com"},
				},
			},
			map[string]*pb.IdentityProvider{
				"cluster": {Issuer: "kubernetes:///", Audiences: []string{"pomerium"}},
				"github": {
					Issuer:        "https://token.actions.githubusercontent.com",
					JwksUrl:       "https://token.actions.githubusercontent.com/.well-known/jwks",
					SupportedAlgs: []string{"RS256", "ES256"},
					Audiences:     []string{"https://pomerium.example.com"},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dst := new(pb.Config)
			err := pomerium.ApplyConfig(t.Context(), dst, &model.Config{
				Pomerium: v1.Pomerium{
					Spec: v1.PomeriumSpec{IdentityProviders: tc.src},
				},
			})
			require.NoError(t, err)
			assert.Empty(t, cmp.Diff(tc.expect, dst.GetSettings().GetIdentityProviders(), protocmp.Transform()))
		})
	}
}

// TestApplyConfig_IdentityProvidersRoundTrip feeds the generated settings back
// through Pomerium's own config to check that what we emit is actually accepted -
// the proto round trip alone does not show whether Pomerium can read the result.
// Note this covers the settings only: Pomerium also cross-validates providers
// against the routes referencing them, which routes do not reach from here.
func TestApplyConfig_IdentityProvidersRoundTrip(t *testing.T) {
	t.Parallel()

	dst := new(pb.Config)
	require.NoError(t, pomerium.ApplyConfig(t.Context(), dst, &model.Config{
		Pomerium: v1.Pomerium{Spec: v1.PomeriumSpec{
			BearerTokenFormat: new("jwt"),
			IdentityProviders: map[string]v1.JWTIdentityProvider{
				"cluster": {Issuer: "kubernetes:///", Audiences: []string{"pomerium"}},
			},
		}},
	}))

	options := config.NewDefaultOptions()
	options.ApplySettings(t.Context(), nil, dst.Settings)

	assert.Equal(t, config.IdentityProvider{
		Issuer:    "kubernetes:///",
		Audiences: []string{"pomerium"},
	}, options.IdentityProviders["cluster"])
	assert.True(t, options.BearerTokenFormat.IsSet)
	assert.Equal(t, pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_JWT, options.BearerTokenFormat.Value)
	assert.NoError(t, options.Validate())
}

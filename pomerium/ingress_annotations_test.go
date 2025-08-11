package pomerium

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

var testPPL = `{"allow":{"or":[{"domain":{"is":"pomerium.com"}}]}}`

func TestAnnotations(t *testing.T) {
	strp := func(txt string) *string { return &txt }
	r := &pb.Route{To: []string{"http://upstream.svc.cluster.local"}}
	ic := &model.IngressConfig{
		AnnotationPrefix: "a",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: v1.ObjectMeta{
				Namespace: "test",
				Annotations: map[string]string{
					"a/allow_any_authenticated_user":            "false",
					"a/allow_public_unauthenticated_access":     "false",
					"a/allow_spdy":                              "true",
					"a/allow_websockets":                        "true",
					"a/allowed_domains":                         `["a"]`,
					"a/allowed_idp_claims":                      `key: ["val1", "val2"]`,
					"a/allowed_users":                           `["a"]`,
					"a/bearer_token_format":                     `idp_access_token`,
					"a/circuit_breaker_thresholds":              `{"max_connections": 1, "max_pending_requests": 2, "max_requests": 3, "max_retries": 4, "max_connection_pools": 5}`,
					"a/cors_allow_preflight":                    "true",
					"a/description":                             "DESCRIPTION",
					"a/depends_on":                              `["foo.example.com", "bar.example.com", "baz.example.com"]`,
					"a/health_checks":                           `[{"timeout": "10s", "interval": "1m", "healthy_threshold": 1, "unhealthy_threshold": 2, "http_health_check": {"path": "/"}}]`,
					"a/host_path_regex_rewrite_pattern":         "rewrite-pattern",
					"a/host_path_regex_rewrite_substitution":    "rewrite-sub",
					"a/host_rewrite_header":                     "rewrite-header",
					"a/host_rewrite":                            "rewrite",
					"a/idle_timeout":                            `60s`,
					"a/idp_access_token_allowed_audiences":      `["x","y","z"]`,
					"a/kubernetes_service_account_token_secret": "k8s_token",
					"a/lb_policy":                               "LEAST_REQUEST",
					"a/logo_url":                                "LOGO_URL",
					"a/least_request_lb_config":                 `{"choice_count":3,"active_request_bias":{"default_value":4,"runtime_key":"key"},"slow_start_config":{"slow_start_window":"3s","aggression":{"runtime_key":"key"}}}`,
					"a/pass_identity_headers":                   "true",
					"a/policy":                                  testPPL,
					"a/prefix_rewrite":                          "/",
					"a/preserve_host_header":                    "true",
					"a/regex_rewrite_pattern":                   `^/service/([^/]+)(/.*)$`,
					"a/regex_rewrite_substitution":              `\\2/instance/\\1`,
					"a/remove_request_headers":                  `["a"]`,
					"a/rewrite_response_headers":                `[{"header": "a", "prefix": "b", "value": "c"}]`,
					"a/secure_upstream":                         "true",
					"a/set_request_headers_secret":              `request_headers`,
					"a/set_request_headers":                     `{"a": "aaa"}`,
					"a/set_response_headers_secret":             `response_headers`,
					"a/set_response_headers":                    `{"disable": true}`,
					"a/timeout":                                 `2m`,
					"a/tls_client_secret":                       "my_client_secret",
					"a/tls_custom_ca_secret":                    "my_custom_ca_secret",
					"a/tls_downstream_client_ca_secret":         "my_downstream_client_ca_secret",
					"a/tls_server_name":                         "my.server.name",
					"a/tls_skip_verify":                         "true",
				},
			},
		},
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "my_custom_ca_secret", Namespace: "test"}: {
				Data: map[string][]byte{
					model.CAKey: []byte("my_custom_ca_secret+cert"),
				},
			},
			{Name: "my_client_secret", Namespace: "test"}: {
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					corev1.TLSCertKey:       []byte("my_client_secret+cert"),
					corev1.TLSPrivateKeyKey: []byte("my_client_secret+key"),
				},
			},
			{Name: "my_downstream_client_ca_secret", Namespace: "test"}: {
				Data: map[string][]byte{
					model.CAKey: []byte("my_downstream_client_ca_secret+cert"),
				},
			},
			{Name: "k8s_token", Namespace: "test"}: {
				Data: map[string][]byte{
					model.KubernetesServiceAccountTokenSecretKey: []byte("k8s-token-data"),
				},
				Type: corev1.SecretTypeServiceAccountToken,
			},
			{Name: "request_headers", Namespace: "test"}: {
				Data: map[string][]byte{
					"req_key_1": []byte("req_data1"),
					"req_key_2": []byte("req_data2"),
				},
				Type: corev1.SecretTypeOpaque,
			},
			{Name: "response_headers", Namespace: "test"}: {
				Data: map[string][]byte{
					"res_key_1": []byte("res_data1"),
					"res_key_2": []byte("res_data2"),
				},
				Type: corev1.SecretTypeOpaque,
			},
		},
	}
	require.NoError(t, applyAnnotations(r, ic))
	require.Empty(t, cmp.Diff(r, &pb.Route{
		TlsCustomCa:                      base64.StdEncoding.EncodeToString([]byte("my_custom_ca_secret+cert")),
		TlsDownstreamClientCa:            base64.StdEncoding.EncodeToString([]byte("my_downstream_client_ca_secret+cert")),
		TlsClientCert:                    base64.StdEncoding.EncodeToString([]byte("my_client_secret+cert")),
		TlsClientKey:                     base64.StdEncoding.EncodeToString([]byte("my_client_secret+key")),
		CorsAllowPreflight:               true,
		AllowPublicUnauthenticatedAccess: false,
		AllowAnyAuthenticatedUser:        false,
		Timeout:                          durationpb.New(time.Minute * 2),
		IdleTimeout:                      durationpb.New(time.Minute),
		AllowWebsockets:                  true,
		AllowSpdy:                        true,
		KubernetesServiceAccountToken:    "k8s-token-data",
		SetRequestHeaders:                map[string]string{"a": "aaa", "req_key_1": "req_data1", "req_key_2": "req_data2"},
		RemoveRequestHeaders:             []string{"a"},
		SetResponseHeaders:               map[string]string{"disable": "true", "res_key_1": "res_data1", "res_key_2": "res_data2"},
		RewriteResponseHeaders: []*pb.RouteRewriteHeader{{
			Header:  "a",
			Matcher: &pb.RouteRewriteHeader_Prefix{Prefix: "b"},
			Value:   "c",
		}},
		PreserveHostHeader:               true,
		HostRewrite:                      strp("rewrite"),
		HostRewriteHeader:                strp("rewrite-header"),
		HostPathRegexRewritePattern:      strp("rewrite-pattern"),
		HostPathRegexRewriteSubstitution: strp("rewrite-sub"),
		PassIdentityHeaders:              proto.Bool(true),
		PrefixRewrite:                    "/",
		RegexRewritePattern:              `^/service/([^/]+)(/.*)$`,
		RegexRewriteSubstitution:         `\\2/instance/\\1`,
		EnvoyOpts: &envoy_config_cluster_v3.Cluster{
			HealthChecks: []*envoy_config_core_v3.HealthCheck{{
				Timeout:            durationpb.New(time.Second * 10),
				Interval:           durationpb.New(time.Minute),
				UnhealthyThreshold: wrapperspb.UInt32(2),
				HealthyThreshold:   wrapperspb.UInt32(1),
				HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{Path: "/"},
				},
			}},
			LbPolicy: envoy_config_cluster_v3.Cluster_LEAST_REQUEST,
			LbConfig: &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig_{
				LeastRequestLbConfig: &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig{
					ChoiceCount: &wrapperspb.UInt32Value{
						Value: 3,
					},
					ActiveRequestBias: &envoy_config_core_v3.RuntimeDouble{
						DefaultValue: 4,
						RuntimeKey:   "key",
					},
					SlowStartConfig: &envoy_config_cluster_v3.Cluster_SlowStartConfig{
						SlowStartWindow: &durationpb.Duration{
							Seconds: 3,
							Nanos:   0,
						},
						Aggression: &envoy_config_core_v3.RuntimeDouble{
							DefaultValue: 0,
							RuntimeKey:   "key",
						},
					},
				},
			},
		},
		Policies: []*pb.Policy{{
			AllowedUsers:   []string{"a"},
			AllowedDomains: []string{"a"},
			AllowedIdpClaims: map[string]*structpb.ListValue{
				"key": {Values: []*structpb.Value{structpb.NewStringValue("val1"), structpb.NewStringValue("val2")}},
			},
			SourcePpl: proto.String(`{"allow":{"or":[{"domain":{"is":"pomerium.com"}}]}}`),
		}},
		TlsSkipVerify:                  true,
		TlsServerName:                  "my.server.name",
		Description:                    "DESCRIPTION",
		LogoUrl:                        "LOGO_URL",
		BearerTokenFormat:              pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_IDP_ACCESS_TOKEN.Enum(),
		IdpAccessTokenAllowedAudiences: &pb.Route_StringList{Values: []string{"x", "y", "z"}},
		DependsOn:                      []string{"foo.example.com", "bar.example.com", "baz.example.com"},
		CircuitBreakerThresholds: &pb.CircuitBreakerThresholds{
			MaxConnections:     proto.Uint32(1),
			MaxPendingRequests: proto.Uint32(2),
			MaxRequests:        proto.Uint32(3),
			MaxRetries:         proto.Uint32(4),
			MaxConnectionPools: proto.Uint32(5),
		},
	}, cmpopts.IgnoreUnexported(
		pb.Route{},
		pb.Route_StringList{},
		pb.RouteRewriteHeader{},
		pb.Policy{},
		structpb.ListValue{},
		structpb.Value{},
		durationpb.Duration{},
		envoy_config_cluster_v3.Cluster{},
		envoy_config_core_v3.HealthCheck{},
		envoy_config_core_v3.HealthCheck_HttpHealthCheck_{},
		envoy_config_core_v3.HealthCheck_HttpHealthCheck{},
		envoy_config_cluster_v3.Cluster_LeastRequestLbConfig{},
		envoy_config_core_v3.RuntimeDouble{},
		envoy_config_cluster_v3.Cluster_CommonLbConfig{},
		envoy_config_cluster_v3.Cluster_SlowStartConfig{},
		wrapperspb.UInt32Value{},
		pb.CircuitBreakerThresholds{},
	),
		cmpopts.IgnoreFields(pb.Policy{}, "Rego")))
	require.NotEmpty(t, r.Policies[0].Rego)
}

func TestMissingTlsAnnotationsSecretData(t *testing.T) {
	r := &pb.Route{To: []string{"http://upstream.svc.cluster.local"}}
	ic := &model.IngressConfig{
		AnnotationPrefix: "a",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: v1.ObjectMeta{
				Namespace: "test",
			},
		},
	}
	require.NoError(t, applyAnnotations(r, ic))

	for name, keys := range map[string][]string{
		"tls_custom_ca_secret":            {model.CAKey},
		"tls_downstream_client_ca_secret": {model.CAKey},
		"tls_client_secret":               {corev1.TLSCertKey, corev1.TLSPrivateKeyKey},
	} {
		ic.Ingress.Annotations = map[string]string{
			fmt.Sprintf("%s/%s", ic.AnnotationPrefix, name): name,
		}
		for _, testKey := range keys {
			data := make(map[string][]byte)
			ic.Secrets = map[types.NamespacedName]*corev1.Secret{
				{Name: name, Namespace: "test"}: {Data: data},
			}
			for _, key := range keys {
				if key == testKey {
					continue
				}
				data[key] = []byte("data")
			}
			assert.Errorf(t, applyAnnotations(r, ic), "name=%s key=%s", name, testKey)
		}
	}
}

func TestYaml(t *testing.T) {
	for input, expect := range map[string]interface{}{
		"10":                                10,
		"0.5":                               0.5,
		"true":                              true,
		"text":                              "text",
		"'text2'":                           "text2",
		`{"bool_key":true}`:                 map[string]interface{}{"bool_key": true},
		`{"groups":["admin", "superuser"]}`: map[string]interface{}{"groups": []interface{}{"admin", "superuser"}},
		`groups: ["admin", "superuser"]`:    map[string]interface{}{"groups": []interface{}{"admin", "superuser"}},
	} {
		var out interface{}
		if assert.NoError(t, yaml.Unmarshal([]byte(input), &out), input) {
			assert.Equal(t, expect, out)
		}
	}
}

func TestMCPAnnotations(t *testing.T) {
	t.Run("MCP Server with OAuth2", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server":                           "true",
						"a/mcp_server_max_request_bytes":         "1048576",
						"a/mcp_server_path":                      "/api/mcp",
						"a/mcp_server_upstream_oauth2_secret":    "mcp-oauth2-secret",
						"a/mcp_server_upstream_oauth2_token_url": "https://auth.example.com/token",
						"a/mcp_server_upstream_oauth2_auth_url":  "https://auth.example.com/auth",
						"a/mcp_server_upstream_oauth2_scopes":    "read,write,admin",
					},
				},
			},
			Secrets: map[types.NamespacedName]*corev1.Secret{
				{Name: "mcp-oauth2-secret", Namespace: "test"}: {
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						model.MCPServerUpstreamOAuth2ClientIDKey:     []byte("test-client-id"),
						model.MCPServerUpstreamOAuth2ClientSecretKey: []byte("test-client-secret"),
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		require.NotNil(t, r.Mcp.GetServer())

		server := r.Mcp.GetServer()
		require.NotNil(t, server.MaxRequestBytes)
		assert.Equal(t, uint32(1048576), *server.MaxRequestBytes)

		assert.Equal(t, "/api/mcp", server.GetPath())

		require.NotNil(t, server.UpstreamOauth2)
		assert.Equal(t, "test-client-id", server.UpstreamOauth2.ClientId)
		assert.Equal(t, "test-client-secret", server.UpstreamOauth2.ClientSecret)
		require.NotNil(t, server.UpstreamOauth2.Oauth2Endpoint)
		assert.Equal(t, "https://auth.example.com/token", server.UpstreamOauth2.Oauth2Endpoint.TokenUrl)
		assert.Equal(t, "https://auth.example.com/auth", server.UpstreamOauth2.Oauth2Endpoint.AuthUrl)
		assert.Equal(t, []string{"read", "write", "admin"}, server.UpstreamOauth2.Scopes)
	})

	t.Run("MCP Client", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-client.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_client": "true",
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		require.NotNil(t, r.Mcp.GetClient())
		assert.Nil(t, r.Mcp.GetServer())
	})

	t.Run("MCP Server minimal", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server": "true",
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		require.NotNil(t, r.Mcp.GetServer())

		server := r.Mcp.GetServer()
		assert.Nil(t, server.MaxRequestBytes)
		assert.Nil(t, server.UpstreamOauth2)
	})

	t.Run("Both server and client should fail", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server": "true",
						"a/mcp_client": "true",
					},
				},
			},
		}

		err := applyAnnotations(r, ic)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot specify both MCP server and client configurations")
	})

	t.Run("Invalid max_request_bytes should fail", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server":                   "true",
						"a/mcp_server_max_request_bytes": "invalid",
					},
				},
			},
		}

		err := applyAnnotations(r, ic)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid max_request_bytes value")
	})

	t.Run("Scopes with whitespace should be trimmed", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server":                        "true",
						"a/mcp_server_upstream_oauth2_scopes": " read , write , admin ",
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		server := r.Mcp.GetServer()
		require.NotNil(t, server.UpstreamOauth2)
		assert.Equal(t, []string{"read", "write", "admin"}, server.UpstreamOauth2.Scopes)
	})

	t.Run("MCP Server OAuth2 secret with only client ID", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server":                        "true",
						"a/mcp_server_upstream_oauth2_secret": "oauth2-clientid-only",
					},
				},
			},
			Secrets: map[types.NamespacedName]*corev1.Secret{
				{Name: "oauth2-clientid-only", Namespace: "test"}: {
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						model.MCPServerUpstreamOAuth2ClientIDKey: []byte("client-id-only"),
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		server := r.Mcp.GetServer()
		require.NotNil(t, server.UpstreamOauth2)
		assert.Equal(t, "client-id-only", server.UpstreamOauth2.ClientId)
		assert.Equal(t, "", server.UpstreamOauth2.ClientSecret)
	})

	t.Run("MCP Server OAuth2 secret with only client secret", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server":                        "true",
						"a/mcp_server_upstream_oauth2_secret": "oauth2-secret-only",
					},
				},
			},
			Secrets: map[types.NamespacedName]*corev1.Secret{
				{Name: "oauth2-secret-only", Namespace: "test"}: {
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						model.MCPServerUpstreamOAuth2ClientSecretKey: []byte("client-secret-only"),
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		server := r.Mcp.GetServer()
		require.NotNil(t, server.UpstreamOauth2)
		assert.Equal(t, "", server.UpstreamOauth2.ClientId)
		assert.Equal(t, "client-secret-only", server.UpstreamOauth2.ClientSecret)
	})

	t.Run("MCP Server OAuth2 secret with no valid keys should fail", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server":                        "true",
						"a/mcp_server_upstream_oauth2_secret": "invalid-oauth2-secret",
					},
				},
			},
			Secrets: map[types.NamespacedName]*corev1.Secret{
				{Name: "invalid-oauth2-secret", Namespace: "test"}: {
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"wrong_key": []byte("wrong-value"),
					},
				},
			},
		}

		err := applyAnnotations(r, ic)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "secret must have at least one of client_id or client_secret keys")
	})

	t.Run("MCP Server implicit via nested annotations", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server_max_request_bytes": "2097152",
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		require.NotNil(t, r.Mcp.GetServer())

		server := r.Mcp.GetServer()
		require.NotNil(t, server.MaxRequestBytes)
		assert.Equal(t, uint32(2097152), *server.MaxRequestBytes)
	})

	t.Run("MCP Server with path", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server":      "true",
						"a/mcp_server_path": "/api/v1/mcp",
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		require.NotNil(t, r.Mcp.GetServer())

		server := r.Mcp.GetServer()
		require.NotNil(t, server.Path)
		assert.Equal(t, "/api/v1/mcp", *server.Path)
	})

	t.Run("MCP Server with default path", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server": "true",
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp.GetServer())
		require.Equal(t, "", r.Mcp.GetServer().GetPath())
		require.Nil(t, r.Mcp.GetServer().Path)
	})

	t.Run("MCP Server with just path", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server_path": "/api/v1/mcp",
					},
				},
			},
		}

		require.NoError(t, applyAnnotations(r, ic))
		require.NotNil(t, r.Mcp)
		require.NotNil(t, r.Mcp.GetServer())

		server := r.Mcp.GetServer()
		require.NotNil(t, server.Path)
		assert.Equal(t, "/api/v1/mcp", *server.Path)
	})
	t.Run("Invalid mcp_server value should fail", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-server.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_server": "https://mcp-server.example.com",
					},
				},
			},
		}

		err := applyAnnotations(r, ic)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mcp_server annotation should be 'true' or omitted")
	})

	t.Run("Invalid mcp_client value should fail", func(t *testing.T) {
		r := &pb.Route{To: []string{"http://mcp-client.example.com"}}
		ic := &model.IngressConfig{
			AnnotationPrefix: "a",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
					Annotations: map[string]string{
						"a/mcp_client": "https://mcp-client.example.com",
					},
				},
			},
		}

		err := applyAnnotations(r, ic)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mcp_client annotation should be 'true' or omitted")
	})
}

func TestCommonLbAnnotation(t *testing.T) {
	t.Run("healthy_panic_threshold annotation", func(t *testing.T) {
		type testcase struct {
			input    float64
			expected float64
		}

		tcs := []testcase{
			{
				input:    50,
				expected: 50,
			},
			{
				input:    -15.0,
				expected: -15.0,
			},
			{
				input:    75.0,
				expected: 75.0,
			},
		}

		for _, tc := range tcs {
			r := &pb.Route{}
			ic := &model.IngressConfig{
				AnnotationPrefix: "a",
				Ingress: &networkingv1.Ingress{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-ingress",
						Namespace: "default",
						Annotations: map[string]string{
							"a/healthy_panic_threshold": fmt.Sprintf("%f", tc.input),
						},
					},
				},
			}
			err := applyAnnotations(r, ic)
			require.NoError(t, err)

			require.NotNil(t, r.EnvoyOpts)
			require.NotNil(t, r.EnvoyOpts.CommonLbConfig)
			require.NotNil(t, r.EnvoyOpts.CommonLbConfig.HealthyPanicThreshold)
			require.Equal(t, r.EnvoyOpts.CommonLbConfig.HealthyPanicThreshold.Value, tc.expected)
		}
	})
}

func TestNameAnnotation(t *testing.T) {
	t.Run("custom name annotation", func(t *testing.T) {
		r := &pb.Route{}
		ic := &model.IngressConfig{
			AnnotationPrefix: "ingress.pomerium.io",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						"ingress.pomerium.io/name": "My Custom Route Name",
					},
				},
			},
		}

		err := applyAnnotations(r, ic)
		require.NoError(t, err)
		assert.Equal(t, "My Custom Route Name", r.Name)
	})

	t.Run("no name annotation uses generated name", func(t *testing.T) {
		r := &pb.Route{}
		ic := &model.IngressConfig{
			AnnotationPrefix: "ingress.pomerium.io",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						"ingress.pomerium.io/allow_any_authenticated_user": "true",
					},
				},
			},
		}

		err := applyAnnotations(r, ic)
		require.NoError(t, err)
		// Name should be empty here since setRouteNameID hasn't been called yet
		assert.Equal(t, "", r.Name)
	})
}

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
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

var (
	testPPL = `{"allow":{"or":[{"domain":{"is":"pomerium.com"}}]}}`
)

func TestAnnotations(t *testing.T) {
	strp := func(txt string) *string { return &txt }
	r := &pb.Route{To: []string{"http://upstream.svc.cluster.local"}}
	ic := &model.IngressConfig{
		AnnotationPrefix: "a",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: v1.ObjectMeta{
				Namespace: "test",
				Annotations: map[string]string{
					"a/allowed_users":                        `["a"]`,
					"a/allowed_groups":                       `["a"]`,
					"a/allowed_domains":                      `["a"]`,
					"a/allowed_idp_claims":                   `{"key": ["val1", "val2"]}`,
					"a/policy":                               testPPL,
					"a/cors_allow_preflight":                 "true",
					"a/allow_public_unauthenticated_access":  "false",
					"a/allow_any_authenticated_user":         "false",
					"a/timeout":                              `10s`,
					"a/idle_timeout":                         `60s`,
					"a/allow_websockets":                     "true",
					"a/set_request_headers":                  `{"a": "aaa"}`,
					"a/remove_request_headers":               `["a"]`,
					"a/set_response_headers":                 `{"c": "ccc"}`,
					"a/rewrite_response_headers":             `[{"header": "a", "prefix": "b", "value": "c"}]`,
					"a/preserve_host_header":                 "true",
					"a/host_rewrite":                         "rewrite",
					"a/host_rewrite_header":                  "rewrite-header",
					"a/host_path_regex_rewrite_pattern":      "rewrite-pattern",
					"a/host_path_regex_rewrite_substitution": "rewrite-sub",
					"a/pass_identity_headers":                "true",
					"a/health_checks":                        `[{"timeout": "10s", "interval": "60s", "healthy_threshold": 1, "unhealthy_threshold": 2, "http_health_check": {"path": "/"}}]`,
					"a/tls_skip_verify":                      "true",
					"a/tls_server_name":                      "my.server.name",
					"a/tls_custom_ca_secret":                 "my_custom_ca_secret",
					"a/tls_client_secret":                    "my_client_secret",
					"a/tls_downstream_client_ca_secret":      "my_downstream_client_ca_secret",
					"a/secure_upstream":                      "true",
				},
			},
		},
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "my_custom_ca_secret", Namespace: "test"}: {
				Data: map[string][]byte{
					CAKey: []byte("my_custom_ca_secret+cert"),
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
					CAKey: []byte("my_downstream_client_ca_secret+cert"),
				},
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
		Timeout:                          durationpb.New(time.Second * 10),
		IdleTimeout:                      durationpb.New(time.Minute),
		AllowWebsockets:                  true,
		SetRequestHeaders:                map[string]string{"a": "aaa"},
		RemoveRequestHeaders:             []string{"a"},
		SetResponseHeaders:               map[string]string{"c": "ccc"},
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
		PassIdentityHeaders:              true,
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
		},
		Policies: []*pb.Policy{{
			AllowedUsers:   []string{"a"},
			AllowedGroups:  []string{"a"},
			AllowedDomains: []string{"a"},
			AllowedIdpClaims: map[string]*structpb.ListValue{
				"key": {Values: []*structpb.Value{structpb.NewStringValue("val1"), structpb.NewStringValue("val2")}},
			},
		}},
		TlsSkipVerify: true,
		TlsServerName: "my.server.name",
	}, cmpopts.IgnoreUnexported(
		pb.Route{},
		pb.RouteRewriteHeader{},
		pb.Policy{},
		structpb.ListValue{},
		structpb.Value{},
		durationpb.Duration{},
		envoy_config_cluster_v3.Cluster{},
		envoy_config_core_v3.HealthCheck{},
		envoy_config_core_v3.HealthCheck_HttpHealthCheck_{},
		envoy_config_core_v3.HealthCheck_HttpHealthCheck{},
		wrapperspb.UInt32Value{},
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
		"tls_custom_ca_secret":            {CAKey},
		"tls_downstream_client_ca_secret": {CAKey},
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

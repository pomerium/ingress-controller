package pomerium

import (
	"testing"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

var (
	testPPL = `{"allow":{"or":[{"domain":{"is":"pomerium.com"}}]}}`
)

func TestAnnotations(t *testing.T) {
	ann := map[string]string{
		"a/allowed_users":                       `["a"]`,
		"a/allowed_groups":                      `["a"]`,
		"a/allowed_domains":                     `["a"]`,
		"a/allowed_idp_claims":                  `{"key": ["val1", "val2"]}`,
		"a/policy":                              testPPL,
		"a/cors_allow_preflight":                "true",
		"a/allow_public_unauthenticated_access": "false",
		"a/allow_any_authenticated_user":        "false",
		"a/timeout":                             `10s`,
		"a/idle_timeout":                        `60s`,
		"a/allow_websockets":                    "true",
		"a/set_request_headers":                 `{"a": "aaa"}`,
		"a/remove_request_headers":              `["a"]`,
		"a/set_response_headers":                `{"c": "ccc"}`,
		"a/rewrite_response_headers":            `[{"header": "a", "prefix": "b", "value": "c"}]`,
		"a/preserve_host_header":                "true",
		"a/pass_identity_headers":               "true",
		"a/health_checks":                       `[{"timeout": "10s", "interval": "60s", "healthy_threshold": 1, "unhealthy_threshold": 2, "http_health_check": {"path": "/"}}]`,
	}
	r := new(pb.Route)
	require.NoError(t, applyAnnotations(r, ann, "a"))
	require.Empty(t, cmp.Diff(r, &pb.Route{
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
		PreserveHostHeader:  true,
		PassIdentityHeaders: true,
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

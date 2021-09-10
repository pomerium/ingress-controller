package pomerium

import (
	"encoding/json"
	"fmt"
	"strings"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"

	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
)

var (
	baseAnnotations = boolMap([]string{
		"allowed_users",
		"allowed_groups",
		"allowed_domains",
		"allowed_idp_claims",
		"cors_allow_preflight",
		"allow_public_unauthenticated_access",
		"allow_any_authenticated_user",
		"timeout",
		"idle_timeout",
		"allow_websockets",
		"set_request_headers",
		"remove_request_headers",
		"set_response_headers",
		"rewrite_response_headers",
		"preserve_host_header",
		"pass_identity_headers",
	})
	envoyAnnotations = boolMap([]string{
		"health_checks",
		"outlier_detection",
		"lb_config",
	})
)

func boolMap(keys []string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

type keys struct {
	Base, Envoy map[string]string
}

func removeKeyPrefix(src map[string]string, prefix string) (*keys, error) {
	prefix = fmt.Sprintf("%s/", prefix)
	kv := keys{
		Base:  make(map[string]string),
		Envoy: make(map[string]string),
	}
	for k, v := range src {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		k = strings.TrimPrefix(k, prefix)
		if baseAnnotations[k] {
			kv.Base[k] = v
		} else if envoyAnnotations[k] {
			kv.Envoy[k] = v
		} else {
			return nil, fmt.Errorf("unknown %s%s", prefix, k)
		}
	}
	return &kv, nil
}

func toJSON(src map[string]string) ([]byte, error) {
	var b strings.Builder
	for k, v := range src {
		_, _ = b.WriteString(k)
		_, _ = b.WriteString(": ")
		_, _ = b.WriteString(v)
		_, _ = b.WriteRune('\n')
	}

	y := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(b.String()), y); err != nil {
		return nil, err
	}

	return json.Marshal(y)
}

// applyAnnotations applies ingress annotations to a route
func applyAnnotations(
	r *pomerium.Route,
	annotations map[string]string,
	prefix string,
) error {
	kv, err := removeKeyPrefix(annotations, prefix)
	if err != nil {
		return err
	}

	if err = unmarshallAnnotations(r, kv.Base); err != nil {
		return err
	}
	r.EnvoyOpts = new(envoy_config_cluster_v3.Cluster)
	return unmarshallAnnotations(r.EnvoyOpts, kv.Envoy)
}

func unmarshallAnnotations(m protoreflect.ProtoMessage, kvs map[string]string) error {
	if len(kvs) == 0 {
		return nil
	}

	data, err := toJSON(kvs)
	if err != nil {
		return err
	}

	return (&protojson.UnmarshalOptions{
		DiscardUnknown: false,
	}).Unmarshal(data, m)
}

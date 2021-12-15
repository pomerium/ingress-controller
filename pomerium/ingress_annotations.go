package pomerium

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/open-policy-agent/opa/ast"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/policy"
)

const (
	CAKey = "ca.crt"
)

var (
	baseAnnotations = boolMap([]string{
		"cors_allow_preflight",
		"allow_public_unauthenticated_access",
		"allow_any_authenticated_user",
		"timeout",
		"idle_timeout",
		"allow_spdy",
		"allow_websockets",
		"set_request_headers",
		"remove_request_headers",
		"set_response_headers",
		"rewrite_response_headers",
		"preserve_host_header",
		"host_rewrite",
		"host_rewrite_header",
		"host_path_regex_rewrite_pattern",
		"host_path_regex_rewrite_substitution",
		"pass_identity_headers",
		"tls_skip_verify",
		"tls_server_name",
	})
	policyAnnotations = boolMap([]string{
		"allowed_users",
		"allowed_groups",
		"allowed_domains",
		"allowed_idp_claims",
		"policy",
	})
	envoyAnnotations = boolMap([]string{
		"health_checks",
		"outlier_detection",
		"lb_config",
	})
	tlsAnnotations = boolMap([]string{
		model.TLSCustomCASecret,
		model.TLSClientSecret,
		model.TLSDownstreamClientCASecret,
	})
	handledElsewhere = boolMap([]string{
		model.SecureUpstream,
		model.PathRegex,
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
	Base, Envoy, Policy, TLS, Etc map[string]string
}

func removeKeyPrefix(src map[string]string, prefix string) (*keys, error) {
	prefix = fmt.Sprintf("%s/", prefix)
	kv := keys{
		Base:   make(map[string]string),
		Envoy:  make(map[string]string),
		Policy: make(map[string]string),
		TLS:    make(map[string]string),
		Etc:    make(map[string]string),
	}
	for k, v := range src {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		k = strings.TrimPrefix(k, prefix)
		known := false
		for _, m := range []struct {
			keys map[string]bool
			dst  map[string]string
		}{
			{baseAnnotations, kv.Base},
			{envoyAnnotations, kv.Envoy},
			{policyAnnotations, kv.Policy},
			{tlsAnnotations, kv.TLS},
			{handledElsewhere, kv.Etc},
		} {
			if m.keys[k] {
				m.dst[k] = v
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("unknown %s%s", prefix, k)
		}
	}
	return &kv, nil
}

func toJSON(src map[string]string) ([]byte, error) {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		out := new(interface{})
		if err := yaml.Unmarshal([]byte(v), out); err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		dst[k] = *out
	}

	return json.Marshal(dst)
}

// applyAnnotations applies ingress annotations to a route
func applyAnnotations(
	r *pomerium.Route,
	ic *model.IngressConfig,
) error {
	kv, err := removeKeyPrefix(ic.Ingress.Annotations, ic.AnnotationPrefix)
	if err != nil {
		return err
	}

	if err = unmarshallAnnotations(r, kv.Base); err != nil {
		return err
	}
	r.EnvoyOpts = new(envoy_config_cluster_v3.Cluster)
	if err = unmarshallAnnotations(r.EnvoyOpts, kv.Envoy); err != nil {
		return err
	}
	if err = applyTlsAnnotations(r, kv.TLS, ic.Secrets, ic.Ingress.Namespace); err != nil {
		return err
	}
	p := new(pomerium.Policy)
	r.Policies = []*pomerium.Policy{p}
	if err := unmarshallPolicyAnnotations(p, kv.Policy); err != nil {
		return fmt.Errorf("applying policy annotations: %w", err)
	}
	return nil
}

func unmarshallPolicyAnnotations(p *pomerium.Policy, kvs map[string]string) error {
	ppl, hasPPL := kvs["policy"]
	if hasPPL {
		delete(kvs, "policy")
	}

	if err := unmarshallAnnotations(p, kvs); err != nil {
		return err
	}

	if !hasPPL {
		return nil
	}

	src, err := policy.GenerateRegoFromReader(strings.NewReader(ppl))
	if err != nil {
		return fmt.Errorf("parsing policy: %w", err)
	}

	_, err = ast.ParseModule("policy.rego", src)
	if err != nil && strings.Contains(err.Error(), "package expected") {
		_, err = ast.ParseModule("policy.rego", "package pomerium.policy\n\n"+src)
	}
	if err != nil {
		return fmt.Errorf("invalid custom rego: %w", err)
	}

	p.Rego = []string{src}
	return nil
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

func applyTlsAnnotations(
	r *pomerium.Route,
	kvs map[string]string,
	secrets map[types.NamespacedName]*corev1.Secret,
	namespace string,
) error {
	for k, name := range kvs {
		secret := secrets[types.NamespacedName{Namespace: namespace, Name: name}]
		if secret == nil {
			return fmt.Errorf("annotation %s references secret %s, but the secret wasn't fetched. this is a bug", k, name)
		}
		var err error
		switch k {
		case model.TLSCustomCASecret:
			if r.TlsCustomCa, err = b64(secret, k, CAKey); err != nil {
				return err
			}
		case model.TLSClientSecret:
			if r.TlsClientCert, err = b64(secret, k, corev1.TLSCertKey); err != nil {
				return err
			}
			if r.TlsClientKey, err = b64(secret, k, corev1.TLSPrivateKeyKey); err != nil {
				return err
			}
		case model.TLSDownstreamClientCASecret:
			if r.TlsDownstreamClientCa, err = b64(secret, k, CAKey); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown annotation %s", k)
		}
	}
	return nil
}

func b64(secret *corev1.Secret, annotation, key string) (string, error) {
	data := secret.Data[key]
	if len(data) == 0 {
		return "", fmt.Errorf("annotation %s references secret %s, key %s has no data",
			annotation, secret.Name, CAKey)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

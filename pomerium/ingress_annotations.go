package pomerium

import (
	"encoding/base64"
	"fmt"
	"strings"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/open-policy-agent/opa/ast"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/policy"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
)

var (
	baseAnnotations = boolMap([]string{
		"allow_any_authenticated_user",
		"allow_public_unauthenticated_access",
		"allow_spdy",
		"allow_websockets",
		"cors_allow_preflight",
		"description",
		"host_path_regex_rewrite_pattern",
		"host_path_regex_rewrite_substitution",
		"host_rewrite_header",
		"host_rewrite",
		"idle_timeout",
		"logo_url",
		"pass_identity_headers",
		"prefix_rewrite",
		"preserve_host_header",
		"regex_rewrite_pattern",
		"regex_rewrite_substitution",
		"remove_request_headers",
		"rewrite_response_headers",
		"set_request_headers",
		"set_response_headers",
		"timeout",
		"tls_server_name",
		"tls_skip_verify",
	})
	policyAnnotations = boolMap([]string{
		"allowed_domains",
		"allowed_idp_claims",
		"allowed_users",
		"policy",
	})
	envoyAnnotations = boolMap([]string{
		"health_checks",
		"lb_policy",
		"least_request_lb_config",
		"maglev_lb_config",
		"outlier_detection",
		"ring_hash_lb_config",
	})
	tlsAnnotations = boolMap([]string{
		model.TLSClientSecret,
		model.TLSCustomCASecret,
		model.TLSDownstreamClientCASecret,
	})
	secretAnnotations = boolMap([]string{
		model.KubernetesServiceAccountTokenSecret,
		model.SetRequestHeadersSecret,
		model.SetResponseHeadersSecret,
	})
	handledElsewhere = boolMap([]string{
		model.PathRegex,
		model.SecureUpstream,
		model.TCPUpstream,
		model.UDPUpstream,
		model.UseServiceProxy,
		model.SubtleAllowEmptyHost,
	})
	unsupported = map[string]string{
		"allowed_groups": "https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync",
	}
)

func boolMap(keys []string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

type keys struct {
	Base, Envoy, Policy, TLS, Etc, Secret map[string]string
}

func removeKeyPrefix(src map[string]string, prefix string) (*keys, error) {
	prefix = fmt.Sprintf("%s/", prefix)
	kv := keys{
		Base:   make(map[string]string),
		Envoy:  make(map[string]string),
		Policy: make(map[string]string),
		TLS:    make(map[string]string),
		Etc:    make(map[string]string),
		Secret: make(map[string]string),
	}

	for k, v := range src {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		k = strings.TrimPrefix(k, prefix)

		if help, ok := unsupported[k]; ok {
			return nil, fmt.Errorf("%s%s no longer supported, see %s", prefix, k, help)
		}

		known := false
		for _, m := range []struct {
			keys map[string]bool
			dst  map[string]string
		}{
			{baseAnnotations, kv.Base},
			{envoyAnnotations, kv.Envoy},
			{policyAnnotations, kv.Policy},
			{tlsAnnotations, kv.TLS},
			{secretAnnotations, kv.Secret},
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

// applyAnnotations applies ingress annotations to a route
func applyAnnotations(
	r *pomerium.Route,
	ic *model.IngressConfig,
) error {
	kv, err := removeKeyPrefix(ic.Ingress.Annotations, ic.AnnotationPrefix)
	if err != nil {
		return err
	}

	if err = unmarshalAnnotations(r, kv.Base); err != nil {
		return err
	}
	r.EnvoyOpts = new(envoy_config_cluster_v3.Cluster)
	if err = unmarshalAnnotations(r.EnvoyOpts, kv.Envoy); err != nil {
		return err
	}
	if err = applyTLSAnnotations(r, kv.TLS, ic.Secrets, ic.Ingress.Namespace); err != nil {
		return err
	}
	if err = applySecretAnnotations(r, kv.Secret, ic.Secrets, ic.Ingress.Namespace); err != nil {
		return err
	}
	p := new(pomerium.Policy)
	r.Policies = []*pomerium.Policy{p}
	if err := unmarshalPolicyAnnotations(p, kv.Policy); err != nil {
		return fmt.Errorf("applying policy annotations: %w", err)
	}
	return nil
}

func unmarshalPolicyAnnotations(p *pomerium.Policy, kvs map[string]string) error {
	ppl, hasPPL := kvs["policy"]
	if hasPPL {
		delete(kvs, "policy")
	}

	if err := unmarshalAnnotations(p, kvs); err != nil {
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

	p.SourcePpl = proto.String(ppl)
	p.Rego = []string{src}
	return nil
}

func applyTLSAnnotations(
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
			if r.TlsCustomCa, err = b64(secret, k, model.CAKey); err != nil {
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
			if r.TlsDownstreamClientCa, err = b64(secret, k, model.CAKey); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown annotation %s", k)
		}
	}
	return nil
}

func applySecretAnnotations(
	r *pomerium.Route,
	annotations map[string]string,
	secrets map[types.NamespacedName]*corev1.Secret,
	namespace string,
) error {
	handlers := map[string]struct {
		expectedType corev1.SecretType
		apply        func(data map[string][]byte) error
	}{
		model.KubernetesServiceAccountTokenSecret: {
			corev1.SecretTypeServiceAccountToken,
			func(data map[string][]byte) error {
				token, ok := data[model.KubernetesServiceAccountTokenSecretKey]
				if !ok {
					return fmt.Errorf("secret must have %s key", model.KubernetesServiceAccountTokenSecretKey)
				}
				r.KubernetesServiceAccountToken = string(token)
				return nil
			},
		},
		model.SetRequestHeadersSecret: {
			corev1.SecretTypeOpaque,
			func(data map[string][]byte) error {
				dst, err := util.MergeMaps(r.SetRequestHeaders, data)
				if err != nil {
					return err
				}
				r.SetRequestHeaders = dst
				return nil
			},
		},
		model.SetResponseHeadersSecret: {
			corev1.SecretTypeOpaque,
			func(data map[string][]byte) error {
				dst, err := util.MergeMaps(r.SetResponseHeaders, data)
				if err != nil {
					return err
				}
				r.SetResponseHeaders = dst
				return nil
			},
		},
	}

	for key, secretName := range annotations {
		handler, ok := handlers[key]
		if !ok {
			return fmt.Errorf("unknown annotation %s", key)
		}

		secret, ok := secrets[types.NamespacedName{Namespace: namespace, Name: secretName}]
		if !ok {
			return fmt.Errorf("annotation %s secret was not fetched. this is a bug", key)
		}

		if secret.Type != handler.expectedType {
			return fmt.Errorf("annotation %s secret is expected to have type %s, got %s", key, handler.expectedType, secret.Type)
		}

		if err := handler.apply(secret.Data); err != nil {
			return fmt.Errorf("annotation %s: %w", key, err)
		}
	}

	return nil
}

func b64(secret *corev1.Secret, annotation, key string) (string, error) {
	data := secret.Data[key]
	if len(data) == 0 {
		return "", fmt.Errorf("annotation %s references secret %s, key %s has no data",
			annotation, secret.Name, model.CAKey)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

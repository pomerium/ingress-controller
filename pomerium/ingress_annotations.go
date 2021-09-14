package pomerium

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/policy"
)

var (
	baseAnnotations = boolMap([]string{
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
	return unmarshallPolicyAnnotations(p, kv.Policy)
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
			return fmt.Errorf("annotation %s references secret=%s, but the secret wasn't fetched. this is a bug", k, name)
		}
		cert := base64.StdEncoding.EncodeToString(secret.Data[corev1.TLSCertKey])
		switch k {
		case model.TLSCustomCASecret:
			r.TlsCustomCa = cert
		case model.TLSClientSecret:
			r.TlsClientCert = cert
			r.TlsClientKey = base64.StdEncoding.EncodeToString(secret.Data[corev1.TLSPrivateKeyKey])
		case model.TLSDownstreamClientCASecret:
			r.TlsDownstreamClientCa = cert
		default:
			return fmt.Errorf("unknown annotation %s", k)
		}
	}
	return nil
}

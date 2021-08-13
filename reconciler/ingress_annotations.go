package reconciler

import (
	"encoding/json"
	"fmt"
	"strings"

	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

var (
	allowedAnnotations = func() map[string]bool {
		ok := []string{
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
		}
		out := make(map[string]bool, len(ok))
		for _, k := range ok {
			out[k] = true
		}
		return out
	}()
)

func removeKeyPrefix(src map[string]string, prefix string) (map[string]string, error) {
	prefix = fmt.Sprintf("%s/", prefix)
	dst := make(map[string]string)
	for k, v := range src {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		k = strings.TrimPrefix(k, prefix)
		if !allowedAnnotations[k] {
			return nil, fmt.Errorf("unknown %s%s", prefix, k)
		}
		dst[k] = v
	}
	return dst, nil
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
	annotations, err := removeKeyPrefix(annotations, prefix)
	if err != nil {
		return err
	}
	if len(annotations) == 0 {
		return nil
	}

	data, err := toJSON(annotations)
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return (&protojson.UnmarshalOptions{
		DiscardUnknown: false,
	}).Unmarshal(data, r)
}

package pomerium

import (
	"fmt"

	"github.com/gosimple/slug"
	"gopkg.in/yaml.v3"

	"github.com/pomerium/pomerium/config"
	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/identity"
	"github.com/pomerium/pomerium/pkg/policy/parser"

	"github.com/pomerium/ingress-controller/model"
)

// ingressToPolicy translates Ingress annotations to a Policy proto compatible
// with the unified API.
func ingressToPolicy(ic *model.IngressConfig) (*pomerium.Policy, error) {
	kv, err := removeKeyPrefix(ic.Ingress.Annotations, ic.AnnotationPrefix)
	if err != nil {
		return nil, err
	}

	p := new(pomerium.Policy)
	if err := unmarshalPolicyAnnotations(p, kv.Policy); err != nil {
		return nil, fmt.Errorf("couldn't unmarshal policy annotations: %w", err)
	}

	// Use the same conversion logic from Core to translate the legacy
	// allowlist fields.
	configPolicy := config.Policy{
		AllowedDomains:   p.AllowedDomains,
		AllowedUsers:     p.AllowedUsers,
		AllowedIDPClaims: identity.NewFlattenedClaimsFromPB(p.AllowedIdpClaims),
	}
	// Include any user-defined PPL.
	if p.SourcePpl != nil {
		configPolicy.Policy = new(config.PPLPolicy)
		if err := yaml.Unmarshal([]byte(*p.SourcePpl), configPolicy.Policy); err != nil {
			return nil, fmt.Errorf("couldn't parse PPL policy: %w", err)
		}
	}

	ppl := configPolicy.ToPPL()
	if isNoOpPolicy(ppl) {
		return nil, nil
	}

	pplBytes, err := ppl.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("couldn't marshal translated PPL: %w", err)
	}
	pplString := string(pplBytes)

	// TODO: consider deriving a name based on policy criteria?
	name := slug.Make(fmt.Sprintf("%s %s policy", ic.Namespace, ic.Name))
	return &pomerium.Policy{
		Name:      &name,
		SourcePpl: &pplString,
	}, nil
}

func isNoOpPolicy(ppl *parser.Policy) bool {
	for _, r := range ppl.Rules {
		if len(r.And) > 0 || len(r.Or) > 0 || len(r.Not) > 0 || len(r.Nor) > 0 {
			return false
		}
	}
	return true
}

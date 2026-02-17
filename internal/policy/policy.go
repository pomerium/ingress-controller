package policy

import (
	"fmt"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"google.golang.org/protobuf/proto"

	"github.com/pomerium/pomerium/pkg/policy"
)

// Parse parses PPL into rego.
func Parse(src string) (ppl *string, rego []string, err error) {
	regoSrc, err := policy.GenerateRegoFromReader(strings.NewReader(src))
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't parse policy: %w", err)
	}

	_, err = ast.ParseModule("policy.rego", regoSrc)
	if err != nil && strings.Contains(err.Error(), "package expected") {
		_, err = ast.ParseModule("policy.rego", "package pomerium.policy\n\n"+regoSrc)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("internal error: %w", err)
	}

	return proto.String(src), []string{regoSrc}, nil
}

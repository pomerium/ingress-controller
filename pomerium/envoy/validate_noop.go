//go:build !embed_pomerium
// +build !embed_pomerium

// Package envoy contains functions for working with an embedded envoy binary.
package envoy

import (
	"context"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
)

// Validate validates the bootstrap envoy config.
func Validate(ctx context.Context, bootstrap *envoy_config_bootstrap_v3.Bootstrap, id string) (*ValidateResult, error) {
	return &ValidateResult{
		Valid:   true,
		Message: "NOOP VALIDATION",
	}, nil
}

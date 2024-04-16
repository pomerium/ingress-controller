//go:build tools
// +build tools

package pomerium

import (
	_ "github.com/client9/misspell/cmd/misspell"
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)

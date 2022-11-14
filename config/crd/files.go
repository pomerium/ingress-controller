// Package crd embeds CRD spec
package crd

import _ "embed"

// SettingsCRD is Pomerium CRD Yaml
//
//go:embed bases/ingress.pomerium.io_pomerium.yaml
var SettingsCRD []byte

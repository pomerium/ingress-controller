//go:build darwin && amd64
// +build darwin,amd64

package envoy

import _ "embed" // embed

//go:embed bin/envoy-darwin-amd64
var rawBinary []byte

//go:embed bin/envoy-darwin-amd64.sha256
var rawChecksum string

const (
	enabled = true
)

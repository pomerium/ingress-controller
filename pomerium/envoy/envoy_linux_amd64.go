//go:build linux && amd64
// +build linux,amd64

package envoy

import _ "embed" // embed

//go:embed bin/envoy-linux-amd64
var rawBinary []byte

//go:embed bin/envoy-linux-amd64.sha256
var rawChecksum string

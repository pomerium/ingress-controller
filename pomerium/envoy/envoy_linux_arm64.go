//go:build linux && arm64
// +build linux,arm64

package envoy

import _ "embed" // embed

//go:embed bin/envoy-linux-arm64
var rawBinary []byte

//go:embed bin/envoy-linux-arm64.sha256
var rawChecksum string

//go:build darwin && arm64
// +build darwin,arm64

package envoy

import _ "embed" // embed

//go:embed bin/envoy-darwin-arm64
var rawBinary []byte

//go:embed bin/envoy-darwin-arm64.sha256
var rawChecksum string

//go:embed bin/envoy-darwin-arm64.version
var rawVersion string

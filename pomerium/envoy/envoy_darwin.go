//go:build darwin
// +build darwin

package envoy

import _ "embed" // embed

//go:embed bin/envoy-darwin-amd64
var rawBinary []byte

//go:embed bin/envoy-darwin-amd64.sha256
var rawChecksum string

//go:embed bin/envoy-darwin-amd64.version
var rawVersion string

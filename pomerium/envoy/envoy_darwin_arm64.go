//go:build darwin && arm64
// +build darwin,arm64

package envoy

import _ "embed" // embed

var (
	rawBinary   []byte
	rawChecksum string
)

const (
	enabled = false
)

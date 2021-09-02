package envoy

import (
	_ "embed"
	"strings"
)

//go:embed bin/envoy.sha256
var rawChecksum string

//go:embed bin/envoy.version
var rawVersion string

// Checksum returns the checksum for the embedded envoy binary.
func Checksum() string {
	return strings.Fields(rawChecksum)[0]
}

// FullVersion returns the full version string for envoy.
func FullVersion() string {
	return Version() + "+" + Checksum()
}

// Version returns the envoy version.
func Version() string {
	return strings.TrimSpace(rawVersion)
}

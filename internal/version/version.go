package version

import (
	"strings"
)

var (
	// Version specifies the ingress controller version, set by the compiler.
	Version = "v0.0.0"
	// GitCommit specifies the git commit sha, set by the compiler.
	GitCommit = ""
	// PomeriumVersion specifies the go.mod dependency version, set by the compiler.
	PomeriumVersion = ""
)

// FullVersion returns a version string.
func FullVersion() string {
	var sb strings.Builder
	sb.Grow(len(Version) + len(GitCommit) + len(PomeriumVersion) + len(" pomerium : ") + len(",") + len("+"))
	sb.WriteString(Version)
	if GitCommit != "" {
		sb.WriteString("+")
		sb.WriteString(GitCommit)
	}
	sb.WriteString(",")
	sb.WriteString(" pomerium : ")
	sb.WriteString(PomeriumVersion)
	return sb.String()
}

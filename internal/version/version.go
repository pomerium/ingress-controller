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
	// officialBuild is set to "1" by the compiler only in builds that Pomerium publishes.
	officialBuild = "0"
)

// LicenseStatement returns a one-line license statement with a link to the license text.
func LicenseStatement() string {
	if officialBuild == "1" {
		return "Licensed under the terms of the Pomerium Community Edition License 1.0: https://github.com/pomerium/ingress-controller/blob/main/licenses/POMERIUM-COMMUNITY-LICENSE-1.0.txt"
	}
	return "Licensed under the terms of the Apache License 2.0: https://github.com/pomerium/ingress-controller/blob/main/LICENSE"
}

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

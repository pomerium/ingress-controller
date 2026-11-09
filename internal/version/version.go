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

	officialBuild = "0"
)

// IsOfficialBuild returns true if this is an official build.
func IsOfficialBuild() bool {
	return officialBuild == "1"
}

// License returns the name and URL of the license governing this build.
func License() (name, url string) {
	if IsOfficialBuild() {
		return "Pomerium Community Edition License 1.0",
			"https://github.com/pomerium/ingress-controller/blob/main/licenses/POMERIUM-COMMUNITY-LICENSE-1.0.txt"
	}
	return "Apache License 2.0", "https://github.com/pomerium/ingress-controller/blob/main/LICENSE"
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

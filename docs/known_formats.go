package docs

var (
	knownFormats = map[string]string{
		"uri":       "an URI as parsed by Golang net/url.ParseRequestURI.",
		"hostname":  "a valid representation for an Internet host name, as defined by RFC 1034, section 3.1 [RFC1034].",
		"ipv4":      "an IPv4 IP as parsed by Golang net.ParseIP",
		"ipv6":      "an IPv6 IP as parsed by Golang net.ParseIP",
		"cidr":      "a CIDR as parsed by Golang net.ParseCIDR",
		"byte":      "base64 encoded binary data",
		"date":      `a date string like "2006-01-02" as defined by full-date in RFC3339`,
		"duration":  `a duration string like "22s" as parsed by Golang time.ParseDuration`,
		"date-time": `a date time string like "2014-12-15T19:30:20.000Z" as defined by date-time in RFC3339.`,
	}
)

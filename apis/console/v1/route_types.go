package v1

// +kubebuilder:validation:Optional

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RouteSpec defines a route.
type RouteSpec struct {
	// Policies lists names of the policies within the same kubernetes namespace this route should apply.
	// +kubebuilder:validation:UniqueItems=true
	Policies []string `json:"policies,omitempty"`

	// Host is the route hostname.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=hostname
	Host string `json:"from"`

	// Prefix would match route by path prefix.
	Prefix *string `json:"prefix,omitempty"`
	// Path would match route if the request path is an exact match.
	Path *string `json:"path,omitempty"`
	// Regex would match route if the request path matches RE2 regular expression.
	Regex *RegexRouteMatch `json:"regex,omitempty"`

	// isTCP indicates the route is a TCP tunnel.
	IsTCP *bool `json:"isTCP,omitempty"`
	// websocket if set, indicates websocket protocol would be served over that route.
	Websocket *bool `json:"websocket,omitempty"`
	// spdy if set, enables proxying of SPDY protocol upgrades.
	SPDY *bool `json:"spdy,omitempty"`

	// To defines list of upstream URLs.
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:UniqueItems=true
	To []string `json:"to,omitempty"`
	// ToService defines list of upstream URLs as a reference to Kubernetes service.
	ToService *ToService `json:"toService,omitempty"`
	// Redirect would serve a redirect in case of a match.
	Redirect *Redirect `json:"redirect,omitempty"`

	// Rewrite would rewrite parts of the request before it is sent upstream.
	Rewrite *Rewrite `json:"rewrite,omitempty"`
	// Redirect would modify request headers before they are presented to the upstream.
	RequestHeaders *RequestHeaders `json:"requestHeaders,omitempty"`

	// Timeout specifies per-route global timeout value.
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	// IdleTimeout defines per-route idle timeout value.
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`

	HealthCheck      *HealthCheck      `json:"healthCheck,omitempty"`
	OutlierDetection *OutlierDetection `json:"outlierDetection,omitempty"`

	LoadBalancing *LoadBalancing `json:"loadBalancing,omitempty"`
}

// LoadBalancing defines load balancing method.
type LoadBalancing struct {
	RoundRobin   *RoundRobinLB   `json:"roundRobin,omitempty"`
	LeastRequest *LeastRequestLB `json:"leastRequest,omitempty"`
	RingHash     *RingHashLB     `json:"ringHash,omitempty"`
	Maglev       *MaglevLB       `json:"maglev,omitempty"`
}

// LeastRequestLB load balancing policy.
type LeastRequestLB struct{}

// RoundRobinLB implements a default iterative load balancing policy.
type RoundRobinLB struct{}

// RingHashLB is a hashing load balancer.
type RingHashLB struct{}

// MaglevLB is a hashing load balancer.
type MaglevLB struct{}

// Redirect will produce a redirect response for the matched route.
type Redirect struct {
	// Host will replace host in the URL with the literal value.
	// +kubebuilder:validation:Format=uri
	Host *string `json:"host,omitempty"`
	// Port replaces port in the URL.
	Port *uint32 `json:"port,omitempty"`
	// StripQuery removes the query string.
	StripQuery *bool `json:"stripQuery,omitempty"`
	// responseCode customizes the redirect response code.
	// +kubebuilder:validation:Enum=MOVED_PERMANENTLY;FOUND;SEE_OTHER;TEMPORARY_REDIRECT;PERMANENT_REDIRECT
	ResponseCode *string `json:"responseCode,omitempty"`
}

// ToService is an alternative way to specify upstream server URLs.
// It uses Kubernetes endpoints unless useServiceProxy is set to true.
type ToService struct {
	// Name should is the referenced service. The service must exist in the same namespace.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// Port of the referenced service. A port name or port number is required.
	// +kubebuilder:validation:Required
	Port ServicePort `json:"port"`
	// UseServiceProxy if set to true, would use kube-proxy instead of dynamically watching service endpoints.
	UseServiceProxy *bool `json:"useServiceProxy,omitempty"`
	// useTLS enables TLS when communicating to the service
	UseTLS *bool `json:"useTLS,omitempty"`
}

// ServicePort refers to a port defined by the service, either using named or numerical form.
type ServicePort struct {
	// Name is the name of the port on the Service. This is a mutually exclusive setting with "number".
	Name *string `json:"name"`
	// Number is the numerical port number (e.g. 80) on the Service. This is a mutually exclusive setting with "Name".
	Number *uint32 `json:"port,omitempty"`
}

// RequestHeaders defines the modifications that has to be done to the incoming request headers before they are passed upstream.
type RequestHeaders struct {
	// Set sets specified request headers. You may not modify some of the reserved names such as Host header.
	Set map[string]string `json:"set,omitempty"`
	// Remove removes listen headers from the request.
	Remove []string `json:"remove,omitempty"`
	// PassIdentity would pass user identity information in X-Pomerium-JWT-Assertion header.
	PassIdentity *bool `json:"passIdentity,omitempty"`

	// authorization send a user's identity token through as a bearer token in the Authorization header.
	// Use ACCESS_TOKEN to send the OAuth access token,
	// ID_TOKEN to send the OIDC ID token,
	// or PASS_THROUGH (the default) to leave the Authorization header unchanged from the client when it's not used for Pomerium authentication.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=ACCESS_TOKEN;ID_TOKEN;PASS_THROUGH
	Authorization *string `json:"authorization,omitempty"`
}

// TLS defines TLS settings.
type TLS struct {
	// Upstream sets upstream TLS options.
	Upstream *UpstreamTLS `json:"upstream,omitempty"`
	// Downstream sets downstream TLS options.
	Downstream *DownstreamTLS `json:"downstream,omitempty"`
}

// UpstreamTLS specifies upstream (backend) TLS settings.
type UpstreamTLS struct {
	// ServerName overrides SNI name.
	ServerName *string `json:"serverName,omitempty"`
	// CA uses custom certificate authority to validate the other party server certificate.
	CA *string `json:"ca,omitempty"`
	// SkipVerify if true, would disable TLS checks.
	SkipVerify *bool `json:"skipVerify,omitempty"`
}

// DownstreamTLS allows to customize the downstream (user client) TLS
type DownstreamTLS struct {
	// ServerName would override the SNI matched server name.
	ServerName *string `json:"serverName,omitempty"`
	// CA to validate client certificates.
	CA *string `json:"ca,omitempty"`
}

// RegexRouteMatch would match a route path based on a regular expression.
type RegexRouteMatch struct {
	// Pattern to match in RE2
	Pattern string `json:"pattern"`
	// Priority, if set, sorts routes in descending order for matching
	Priority *int64 `json:"priority,omitempty"`
}

// Rewrite groups rewrite options.
type Rewrite struct {
	Host            *HostRewrite          `json:"host,omitempty"`
	Path            *PathRewrite          `json:"path,omitempty"`
	ResponseHeaders []*RouteRewriteHeader `json:"responseHeaders,omitempty"`
}

// HostRewrite defines Host header behavior during proxying the request.
type HostRewrite struct {
	// Preserve will, when enabled, this option will pass the host header from the incoming request to the proxied host,
	// instead of the destination hostname. It's an optional parameter of type bool that defaults to false.
	Preserve *bool `json:"preserve,omitempty"`
	// Rewrite will rewrite the host to a new literal value.
	Rewrite *string `json:"rewrite,omitempty"`
	// Header will rewrite the host to match an incoming header value.
	Header *string `json:"header,omitempty"`
	// Regex will rewrite the host according to a regex matching the path.
	Regex *HostRewriteRegex `json:"regex,omitempty"`
}

// HostRewriteRegex will rewrite the host according to a regex matching the path
type HostRewriteRegex struct {
	// pattern to match, should be RE2 format including ^ and $
	Pattern string `json:"pattern"`
	// substitution the value to rewrite to
	Substitution string `json:"substitution"`
}

// PathRewrite rewrites the path.
type PathRewrite struct {
	// Prefix replaces path prefix with the provided value.
	Prefix *string `json:"prefix,omitempty"`
	// Regex replaces just the matching part of the path.
	Regex *RegexPathRewrite `json:"regex,omitempty"`
}

// RegexPathRewrite rewrites matching part of the path.
type RegexPathRewrite struct {
	// Pattern to match
	Pattern string `json:"pattern,omitempty"`
	// Substitution value to replace with.
	Substitution string `json:"substitution,omitempty"`
}

// RouteRewriteHeader defines HTTP header rewrite rules.
type RouteRewriteHeader struct {
	// Header key
	Header string `json:"header"`
	// Prefix will be replaced with value.
	Prefix string `json:"prefix"`
	// Literal value to replace with.
	Value string `json:"value"`
}

// HealthCheck defines rules to detect if a particular upstream endpoint is healthy.
type HealthCheck struct {
	// The time to wait for a health check response. If the timeout is reached the
	// health check attempt will be considered a failure.
	// +kubebuilder:validation:Required
	Timeout metav1.Duration `json:"timeout"`
	// The interval between health checks.
	// +kubebuilder:validation:Required
	Interval metav1.Duration `json:"interval"`
	// An optional jitter amount in milliseconds. If specified, Envoy will start health
	// checking after for a random time in ms between 0 and initial_jitter. This only
	// applies to the first health check.
	InitialJitter *metav1.Duration `json:"initialJitter,omitempty"`
	// An optional jitter amount in milliseconds. If specified, during every
	// interval Envoy will add interval_jitter to the wait time.
	IntervalJitter *metav1.Duration `json:"intervalJitter,omitempty"`
	// An optional jitter amount as a percentage of interval_ms. If specified,
	// during every interval Envoy will add interval_ms *
	// interval_jitter_percent / 100 to the wait time.
	//
	// If interval_jitter_ms and interval_jitter_percent are both set, both of
	// them will be used to increase the wait time.
	IntervalJitterPercent *uint32 `json:"intervalJitterPercent,omitempty"`
	// The number of unhealthy health checks required before a host is marked
	// unhealthy. Note that for *http* health checking if a host responds with a code not in
	// expectedStatuses retriableStatuses, this threshold is ignored and the host is considered immediately unhealthy.
	UnhealthyThreshold *uint32 `json:"unhealthyThreshold,omitempty"`
	// The number of healthy health checks required before a host is marked
	// healthy. Note that during startup, only a single successful health check is
	// required to mark a host healthy.
	HealthyThreshold *uint32 `json:"healthyThreshold,omitempty"`
	// Reuse health check connection between health checks. Default is true.
	ReuseConnection *bool `json:"reuseConnection,omitempty"`
	// HTTP health check
	HTTP *HealthCheckHTTP `json:"http"`
	// The "no traffic interval" is a special health check interval that is used when a cluster has
	// never had traffic routed to it. This lower interval allows cluster information to be kept up to
	// date, without sending a potentially large amount of active health checking traffic for no
	// reason. Once a cluster has been used for traffic routing, Envoy will shift back to using the
	// standard health check interval that is defined. Note that this interval takes precedence over
	// any other.
	//
	// The default value for "no traffic interval" is 60 seconds.
	NoTrafficInterval *metav1.Duration `json:"noTrafficInterval,omitempty"`
	// The "no traffic healthy interval" is a special health check interval that
	// is used for hosts that are currently passing active health checking
	// (including new hosts) when the cluster has received no traffic.
	//
	// This is useful for when we want to send frequent health checks with
	// `no_traffic_interval` but then revert to lower frequency `no_traffic_healthy_interval` once
	// a host in the cluster is marked as healthy.
	//
	// Once a cluster has been used for traffic routing, Envoy will shift back to using the
	// standard health check interval that is defined.
	//
	// If no_traffic_healthy_interval is not set, it will default to the
	// no traffic interval and send that interval regardless of health state.
	NoTrafficHealthyInterval *metav1.Duration `json:"noTrafficHealthyInterval,omitempty"`
	// The "unhealthy interval" is a health check interval that is used for hosts that are marked as
	// unhealthy. As soon as the host is marked as healthy, Envoy will shift back to using the
	// standard health check interval that is defined.
	//
	// The default value for "unhealthy interval" is the same as "interval".
	UnhealthyInterval *metav1.Duration `json:"unhealthyInterval,omitempty"`
	// The "unhealthy edge interval" is a special health check interval that is used for the first
	// health check right after a host is marked as unhealthy. For subsequent health checks
	// Envoy will shift back to using either "unhealthy interval" if present or the standard health
	// check interval that is defined.
	//
	// The default value for "unhealthy edge interval" is the same as "unhealthy interval".
	UnhealthyEdgeInterval *metav1.Duration `json:"unhealthyEdgeInterval,omitempty"`
	// The "healthy edge interval" is a special health check interval that is used for the first
	// health check right after a host is marked as healthy. For subsequent health checks
	// Envoy will shift back to using the standard health check interval that is defined.
	//
	// The default value for "healthy edge interval" is the same as the default interval.
	HealthyEdgeInterval *metav1.Duration `json:"healthyEdgeInterval,omitempty"`
	// If set to true, health check failure events will always be logged. If set to false, only the
	// initial health check failure event will be logged.
	// The default value is false.
	AlwaysLogHealthCheckFailures bool `json:"alwaysLogHealthCheckFailures,omitempty"`
}

// HealthCheckHTTP is a HTTP request based health checker.
type HealthCheckHTTP struct {
	// The value of the host header in the HTTP health check request.
	Host *string `json:"host,omitempty"`
	// Specifies the HTTP path that will be requested during health checking.
	// +kubebuilder:validation:Required
	Path string `json:"path,omitempty"`
	// Specifies a list of HTTP headers that should be added to each request that is sent to the
	// health checked cluster. For more information, including details on header value syntax, see
	// the documentation customRequestHeaders
	// +kubebuilder:validation:Optional
	RequestHeadersToAdd []HeaderValueOption `json:"requestHeadersToAdd,omitempty"`
	// Specifies a list of HTTP headers that should be removed from each request that is sent to the
	// health checked cluster.
	// +kubebuilder:validation:Optional
	RequestHeadersToRemove []string `json:"requestHeadersToRemove,omitempty"`
	// Specifies a list of HTTP response statuses considered healthy. If provided, replaces default
	// 200-only policy - 200 must be included explicitly as needed. Ranges follow half-open
	// semantics of [start,end). The start and end of each
	// range are required. Only statuses in the range [100, 600) are allowed.
	ExpectedStatuses []Int64Range `json:"expectedStatuses,omitempty"`
	// Specifies a list of HTTP response statuses considered retriable. If provided, responses in this range
	// will count towards the configured unhealthyThreshold,
	// but will not result in the host being considered immediately unhealthy. Ranges follow half-open semantics of
	// [start,end). The start and end of each range are required.
	// Only statuses in the range [100, 600) are allowed.
	// field takes precedence for any range overlaps with this field i.e. if status code 200 is both retriable and expected, a 200 response will
	// be considered a successful health check. By default all responses not in expectedStatuses will result in
	// the host being considered immediately unhealthy i.e. if status code 200 is expected and there are no configured retriable statuses, any
	// non-200 response will result in the host being marked unhealthy.
	RetriableStatuses []Int64Range `json:"retriableStatuses,omitempty"`
	// Use specified application protocol for health checks.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=HTTP1;HTTP2
	CodecClientType *string `json:"codecClientType,omitempty"`
}

// HeaderValueOption defines a single header modification.
type HeaderValueOption struct {
	// Key is a header key.
	Key string `json:"key"`
	// Value is a header value. If nil, custom headers with empty values are dropped.
	// If value is set to an empty string, the header would be added with an empty value.
	Value *string `json:"value,omitempty"`
	// Should the value be appended? If true (default), the value is appended to
	// existing values. Otherwise it replaces any existing values.
	Append *bool `json:"append,omitempty"`
}

// OutlierDetection enables passive endpoint health check detection based on observed responses.
type OutlierDetection struct {
	// The number of consecutive 5xx responses or local origin errors that are mapped
	// to 5xx error codes before a consecutive 5xx ejection
	// occurs. Defaults to 5.
	Consecutive5Xx *uint32 `json:"consecutive5xx,omitempty"`
	// The time interval between ejection analysis sweeps. This can result in
	// both new ejections as well as hosts being returned to service. Defaults
	// to 10000ms or 10s.
	Interval *metav1.Duration `json:"interval,omitempty"`
	// The base time that a host is ejected for. The real time is equal to the
	// base time multiplied by the number of times the host has been ejected and is
	// capped by maxEjectionTime.
	// Defaults to 30000ms or 30s.
	BaseEjectionTime *metav1.Duration `json:"baseEjectionTime,omitempty"`
	// The maximum % of an upstream cluster that can be ejected due to outlier
	// detection. Defaults to 10% but will eject at least one host regardless of the value.
	MaxEjectionPercent *uint32 `json:"maxEjectionPercent,omitempty"`
	// The % chance that a host will be actually ejected when an outlier status
	// is detected through consecutive 5xx. This setting can be used to disable
	// ejection or to ramp it up slowly. Defaults to 100.
	EnforcingConsecutive5Xx *uint32 `json:"enforcingConsecutive5xx,omitempty"`
	// The % chance that a host will be actually ejected when an outlier status
	// is detected through success rate statistics. This setting can be used to
	// disable ejection or to ramp it up slowly. Defaults to 100.
	EnforcingSuccessRate *uint32 `json:"enforcingSuccessRate,omitempty"`
	// The number of hosts in a cluster that must have enough request volume to
	// detect success rate outliers. If the number of hosts is less than this
	// setting, outlier detection via success rate statistics is not performed
	// for any host in the cluster. Defaults to 5.
	SuccessRateMinimumHosts *uint32 `json:"successRateMinimumHosts,omitempty"`
	// The minimum number of total requests that must be collected in one
	// interval (as defined by the interval duration above) to include this host
	// in success rate based outlier detection. If the volume is lower than this
	// setting, outlier detection via success rate statistics is not performed
	// for that host. Defaults to 100.
	SuccessRateRequestVolume *uint32 `json:"successRateRequestVolume,omitempty"`
	// This factor is used to determine the ejection threshold for success rate
	// outlier ejection. The ejection threshold is the difference between the
	// mean success rate, and the product of this factor and the standard
	// deviation of the mean success rate: mean - (stdev *
	// success_rate_stdev_factor). This factor is divided by a thousand to get a
	// double. That is, if the desired factor is 1.9, the runtime value should
	// be 1900. Defaults to 1900.
	SuccessRateStdevFactor *uint32 `json:"successRateStdevFactor,omitempty"`
	// The number of consecutive gateway failures (502, 503, 504 status codes)
	// before a consecutive gateway failure ejection occurs. Defaults to 5.
	ConsecutiveGatewayFailure *uint32 `json:"consecutiveGatewayFailure,omitempty"`
	// The % chance that a host will be actually ejected when an outlier status
	// is detected through consecutive gateway failures. This setting can be
	// used to disable ejection or to ramp it up slowly. Defaults to 0.
	EnforcingConsecutiveGatewayFailure *uint32 `json:"enforcingConsecutiveGatewayFailure,omitempty"`
	// Determines whether to distinguish local origin failures from external errors. If set to true
	// the following configuration parameters are taken into account:
	// consecutiveLocalOriginFailure, enforcingConsecutiveLocalOriginFailure and enforcingLocalOriginSuccessRate
	// Defaults to false.
	SplitExternalLocalOriginErrors bool `json:"splitExternalLocalOriginErrors,omitempty"`
	// The number of consecutive locally originated failures before ejection
	// occurs. Defaults to 5. Parameter takes effect only when
	// splitExternalLocalOriginErrors is set to true.
	ConsecutiveLocalOriginFailure *uint32 `json:"consecutiveLocalOriginFailure,omitempty"`
	// The % chance that a host will be actually ejected when an outlier status
	// is detected through consecutive locally originated failures. This setting can be
	// used to disable ejection or to ramp it up slowly. Defaults to 100.
	// Parameter takes effect only when splitExternalLocalOriginErrors
	// is set to true.
	EnforcingConsecutiveLocalOriginFailure *uint32 `json:"enforcingConsecutiveLocalOriginFailure,omitempty"`
	// The % chance that a host will be actually ejected when an outlier status
	// is detected through success rate statistics for locally originated errors.
	// This setting can be used to disable ejection or to ramp it up slowly. Defaults to 100.
	// Parameter takes effect only when splitExternalLocalOriginErrors is set to true.
	EnforcingLocalOriginSuccessRate *uint32 `json:"enforcingLocalOriginSuccessRate,omitempty"`
	// The failure percentage to use when determining failure percentage-based outlier detection. If
	// the failure percentage of a given host is greater than or equal to this value, it will be
	// ejected. Defaults to 85.
	FailurePercentageThreshold *uint32 `json:"failurePercentageThreshold,omitempty"`
	// The % chance that a host will be actually ejected when an outlier status is detected through
	// failure percentage statistics. This setting can be used to disable ejection or to ramp it up
	// slowly. Defaults to 0.
	EnforcingFailurePercentage *uint32 `json:"enforcingFailurePercentage,omitempty"`
	// The % chance that a host will be actually ejected when an outlier status is detected through
	// local-origin failure percentage statistics. This setting can be used to disable ejection or to
	// ramp it up slowly. Defaults to 0.
	EnforcingFailurePercentageLocalOrigin *uint32 `json:"enforcingFailurePercentageLocalOrigin,omitempty"`
	// The minimum number of hosts in a cluster in order to perform failure percentage-based ejection.
	// If the total number of hosts in the cluster is less than this value, failure percentage-based
	// ejection will not be performed. Defaults to 5.
	FailurePercentageMinimumHosts *uint32 `json:"failurePercentageMinimumHosts,omitempty"`
	// The minimum number of total requests that must be collected in one interval (as defined by the
	// interval duration above) to perform failure percentage-based ejection for this host. If the
	// volume is lower than this setting, failure percentage-based ejection will not be performed for
	// this host. Defaults to 50.
	FailurePercentageRequestVolume *uint32 `json:"failurePercentageRequestVolume,omitempty"`
	// The maximum time that a host is ejected for. See baseEjectionTime
	// for more information. If not specified, the default value (300000ms or 300s) or
	// baseEjectionTime value is applied, whatever is larger.
	MaxEjectionTime *metav1.Duration `json:"maxEjectionTime,omitempty"`
	// The maximum amount of jitter to add to the ejection time, in order to prevent
	// a 'thundering herd' effect where all proxies try to reconnect to host at the same time.
	// baseEjectionTime. Defaults to 0s.
	MaxEjectionTimeJitter *metav1.Duration `json:"maxEjectionTimeJitter,omitempty"`
}

// Int64Range represents a [start,end) range.
type Int64Range struct {
	// Start of the range (inclusive)
	// +kubebuilder:validation:Required
	Start int64 `json:"start"`
	// End of the range (exclusive)
	// +kubebuilder:validation:Required
	End int64 `json:"end"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=routes

// Route define Pomerium Enterprise Console namespace
type Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteSpec      `json:"spec,omitempty"`
	Status ResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RouteList contains a list of Settings
type RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Namespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Route{}, &RouteList{})
}

package v1

// +kubebuilder:validation:Optional

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicySpec defines Pomerium Enterprise Console namespace
type PolicySpec struct {
	// RefID allows to refer to an existing policy created in the console.
	// Mutually exclusive with any other options.
	RefID *string `json:"refId,omitempty"`

	// Enforced policy automatically applied to all routes in this and subordinate namespaces.
	Enforced *bool `json:"enforced,omitempty"`

	Description *string `json:"description,omitempty"`
	Explanation *string `json:"explanation,omitempty"`
	Remediation *string `json:"remediation,omitempty"`

	// PPL allows to express the policy in Pomerium Policy Language.
	// Mutually exclusive with Rego.
	PPL []PolicyRule `json:"ppl,omitempty"`

	// Rego allows to write raw Rego.
	// Mutually exclusive with PPL.
	Rego *string `json:"rego,omitempty"`
}

// PolicyRule should be either allow or deny.
type PolicyRule struct {
	Allow *PolicyAction `json:"allow,omitempty"`
	Deny  *PolicyAction `json:"deny,omitempty"`
}

// PolicyAction defines blocks of criteria.
// Only one operator may be defined.
type PolicyAction struct {
	And []PolicyCriteria `json:"and,omitempty"`
	Or  []PolicyCriteria `json:"or,omitempty"`
	Not []PolicyCriteria `json:"not,omitempty"`
	Nor []PolicyCriteria `json:"nor,omitempty"`
}

// PolicyCriteria specifies PPL matching criteria.
// Only one criteria may be used.
// see https://www.pomerium.com/docs/topics/ppl for details.
type PolicyCriteria struct {
	Accept                   *bool                `json:"accept,omitempty"`
	AuthenticatedUser        *bool                `json:"authenticated_user,omitempty"`
	Claim                    *PolicyCriteriaClaim `json:"claim,omitempty"`
	CorsAllowPreflight       *bool                `json:"cors_preflight,omitempty"`
	Device                   *DeviceMatcher       `json:"device,omitempty"`
	Domain                   *StringMatcher       `json:"domain,omitempty"`
	Email                    *StringMatcher       `json:"email,omitempty"`
	Groups                   *StringListMatcher   `json:"groups,omitempty"`
	HTTPMethod               *StringMatcher       `json:"http_method,omitempty"`
	HTTPPath                 *StringMatcher       `json:"http_path,omitempty"`
	InvalidClientCertificate *bool                `json:"invalid_client_certificate,omitempty"`
	PomeriumRoutes           *bool                `json:"pomerium_routes,omitempty"`
	Reject                   *bool                `json:"reject,omitempty"`
	User                     *StringMatcher       `json:"user,omitempty"`
	Date                     *DateTimeMatcher     `json:"date,omitempty"`
	DayOfWeek                *string              `json:"day_of_week,omitempty"`
	Record                   *RecordMatcher       `json:"record,omitempty"`
}

// RecordMatcher matches external records
type RecordMatcher struct {
	// +kubebuilder:validation:Required
	Type string `json:"type"`
	// +kubebuilder:validation:Required
	Field             string `json:"field"`
	StringMatcher     `json:","`
	StringListMatcher `json:","`
}

// StringMatcher implements https://www.pomerium.com/docs/topics/ppl#string-matcher
type StringMatcher struct {
	Contains   *string `json:"contains,omitempty"`
	EndsWith   *string `json:"ends_with,omitempty"`
	StartsWith *string `json:"starts_with,omitempty"`
	Is         *string `json:"is,omitempty"`
}

// DateTimeMatcher implements https://www.pomerium.com/docs/topics/ppl#date-matcher
type DateTimeMatcher struct {
	Before *metav1.Time `json:"before,omitempty"`
	After  *metav1.Time `json:"after,omitempty"`
}

// StringListMatcher implements https://www.pomerium.com/docs/topics/ppl#list-matcher
type StringListMatcher struct {
	Has *string `json:"has,omitempty"`
}

// DeviceMatcher implements https://www.pomerium.com/docs/topics/ppl#device-matcher
type DeviceMatcher struct {
	IS       *string `json:"is,omitempty"`
	Approved *bool   `json:"approved,omitempty"`
	// +kubebuilder:validation:Enum=any;enclave_only
	Type *string `json:"type,omitempty"`
}

// PolicyCriteriaClaim defines a criteria of authentication claims
type PolicyCriteriaClaim struct {
	// Path is claim path, i.e. family_name
	// +kubebuilder:validation:Required
	Path string `json:"path"`
	// Value to match.
	// +kubebuilder:validation:Required
	Value string `json:"value"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=policies

// Policy defines Pomerium Policy
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicySpec     `json:"spec,omitempty"`
	Status ResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PolicyList contains a list of Policies
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Policy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
}

package gateway

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

// refKey is an object reference in a form suitable for use as a map key.
// Gateway references have some optional fields with default values that vary by type.
// In a refKey these defaults should be made explicit.
type refKey struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func refKeyForObject(obj client.Object) refKey {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return refKey{
		Group:     gvk.Group,
		Kind:      gvk.Kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func refKeyForParentRef(obj client.Object, ref *gateway_v1.ParentReference) refKey {
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.ParentReference
	// "When unspecified, “gateway.networking.k8s.io” is inferred."
	group := gateway_v1.GroupName
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	// Kind appears to have a default value but I don't see this clearly spelled out in the API
	// reference. I think Gateway is the only kind we care about in practice.
	kind := "Gateway"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	namespace := obj.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return refKey{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      string(ref.Name),
	}
}

func refKeyForCertificateRef(obj client.Object, ref *gateway_v1.SecretObjectReference) refKey {
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.SecretObjectReference
	// "When unspecified or empty string, core API group is inferred."
	group := corev1.GroupName
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	// "SecretObjectReference identifies an API object including its namespace, defaulting to Secret."
	kind := "Secret"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	namespace := obj.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return refKey{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      string(ref.Name),
	}
}

func refKeyForBackendRef(obj client.Object, ref *gateway_v1.BackendObjectReference) refKey {
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.BackendObjectReference
	// "When unspecified or empty string, core API group is inferred."
	group := corev1.GroupName
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	// "Defaults to "Service" when not specified."
	kind := "Service"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	namespace := obj.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return refKey{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      string(ref.Name),
	}
}

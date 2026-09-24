package model

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

// PomeriumServiceKind is the kind of a PomeriumService resource backend.
const PomeriumServiceKind = "PomeriumService"

// IsPomeriumServiceRef reports whether an Ingress resource backend refers to
// a PomeriumService.
func IsPomeriumServiceRef(ref *corev1.TypedLocalObjectReference) bool {
	return ref != nil && ref.APIGroup != nil &&
		*ref.APIGroup == icsv1.GroupVersion.Group && ref.Kind == PomeriumServiceKind
}

// ResourceRefString formats a resource backend reference for error messages.
func ResourceRefString(ref *corev1.TypedLocalObjectReference) string {
	group := ""
	if ref.APIGroup != nil {
		group = *ref.APIGroup
	}
	return fmt.Sprintf("%s/%s %s", group, ref.Kind, ref.Name)
}

package util

import "sigs.k8s.io/controller-runtime/pkg/client"

// SetAnnotation sets the given annotation key/value, making a new Annotations
// map if one does not already exist.
func SetAnnotation(object client.Object, key, value string) {
	m := object.GetAnnotations()
	if m == nil {
		m = make(map[string]string)
	}
	m[key] = value
	object.SetAnnotations(m)
}

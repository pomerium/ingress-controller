package generic

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// GVKForType returns the GroupVersionKind for a given type T registered in the scheme.
// It panics if the type is not registered or there is more than one GVK for the type.
func GVKForType[T client.Object](scheme *runtime.Scheme) schema.GroupVersionKind {
	t := reflect.New(reflect.TypeFor[T]().Elem()).Interface().(T)
	gvk, err := apiutil.GVKForObject(t, scheme)
	if err != nil {
		panic(fmt.Errorf("bug: %w", err))
	}
	return gvk
}

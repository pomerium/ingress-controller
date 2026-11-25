package ingress

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pomerium/ingress-controller/model"
)

func TestEndpointSliceMapsToIngress(t *testing.T) {
	reg := model.NewRegistry()
	reg.Add(
		model.Key{Kind: "Ingress", NamespacedName: types.NamespacedName{Namespace: "default", Name: "ingress"}},
		model.Key{Kind: "Service", NamespacedName: types.NamespacedName{Namespace: "default", Name: "service"}},
	)
	r := &ingressController{Registry: reg, ingressKind: "Ingress", serviceKind: "Service"}

	slice := func(namespace string, labels map[string]string) *discoveryv1.EndpointSlice {
		return &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "service-abcde", Namespace: namespace, Labels: labels}}
	}

	assert.Equal(t,
		[]reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "default", Name: "ingress"}}},
		r.getDependantIngressFromEndpointSlice(context.Background(), slice("default", map[string]string{discoveryv1.LabelServiceName: "service"})),
		"slice of a service the ingress depends on")
	assert.Empty(t,
		r.getDependantIngressFromEndpointSlice(context.Background(), slice("default", map[string]string{discoveryv1.LabelServiceName: "other"})),
		"slice of an unrelated service")
	assert.Empty(t,
		r.getDependantIngressFromEndpointSlice(context.Background(), slice("other", map[string]string{discoveryv1.LabelServiceName: "service"})),
		"slice in another namespace")
	assert.Empty(t,
		r.getDependantIngressFromEndpointSlice(context.Background(), slice("default", nil)),
		"slice without the service-name label")
}

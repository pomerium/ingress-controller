package ingress

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestManagingIngressClass(t *testing.T) {
	pomeriumControllerName := "pomerium.io/ingress-controller"
	pomeriumIngressClass := "pomerium"
	otherIngressClass := "legacy"
	otherControllerName := "legacy.com/ingress"

	ctx := context.Background()
	mc := NewMockClient(gomock.NewController(t))
	ctrl := ingressController{
		controllerName:   pomeriumControllerName,
		annotationPrefix: DefaultAnnotationPrefix,
		Client:           mc,
		endpointsKind:    "Endpoints",
		ingressKind:      "Ingress",
		ingressClassKind: "IngressClass",
		secretKind:       "Secret",
		serviceKind:      "Service",
		initComplete:     newOnce(func(ctx context.Context) error { return nil }),
	}

	testCases := []struct {
		title   string
		ingress networkingv1.Ingress
		classes []networkingv1.IngressClass
		result  bool
	}{
		{
			"our ingress class",
			networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: &pomeriumIngressClass,
				}},
			[]networkingv1.IngressClass{{
				ObjectMeta: metav1.ObjectMeta{
					Name: pomeriumIngressClass,
				},
				Spec: networkingv1.IngressClassSpec{
					Controller: pomeriumControllerName,
				},
			}},
			true,
		},
		{
			"ignore other ingress classes",
			networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					IngressClassName: &otherIngressClass,
				},
			}, []networkingv1.IngressClass{{
				ObjectMeta: metav1.ObjectMeta{
					Name: otherIngressClass,
				},
				Spec: networkingv1.IngressClassSpec{
					Controller: otherControllerName,
				},
			}},
			false,
		},
		{
			"deprecated method used by HTTP solvers",
			networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						IngressClassAnnotationKey: pomeriumIngressClass,
					},
				},
			}, []networkingv1.IngressClass{{
				ObjectMeta: metav1.ObjectMeta{
					Name: pomeriumIngressClass,
				},
				Spec: networkingv1.IngressClassSpec{
					Controller: pomeriumControllerName,
				},
			}},
			true,
		},
		{
			"default ingress", networkingv1.Ingress{}, []networkingv1.IngressClass{{
				ObjectMeta: metav1.ObjectMeta{
					Name: pomeriumIngressClass,
					Annotations: map[string]string{
						IngressClassDefaultAnnotationKey: "true",
					},
				},
				Spec: networkingv1.IngressClassSpec{
					Controller: pomeriumControllerName,
				},
			}},
			true,
		},
	}

	var classes []networkingv1.IngressClass
	mc.EXPECT().List(ctx, gomock.AssignableToTypeOf(&networkingv1.IngressClassList{})).
		Do(func(_ context.Context, dst *networkingv1.IngressClassList, _ ...client.ListOption) {
			dst.Items = classes
		}).
		Return(nil).
		Times(len(testCases))
	for _, tc := range testCases {
		classes = tc.classes
		ok, err := ctrl.isManaging(ctx, &tc.ingress)
		if assert.NoError(t, err, tc.title) {
			assert.Equal(t, tc.result, ok, tc.title)
		}
	}
}

package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManagingIngressClass(t *testing.T) {
	ctx := context.Background()
	pomeriumControllerName := "pomerium.io/ingress-controller"
	pomeriumIngressClass := "pomerium"
	otherIngressClass := "legacy"
	otherControllerName := "legacy.com/ingress"

	assert.True(t, isManagingClass(ctx, &networkingv1.Ingress{
		Spec: networkingv1.IngressSpec{
			IngressClassName: &pomeriumIngressClass,
		},
	}, []networkingv1.IngressClass{{
		ObjectMeta: metav1.ObjectMeta{
			Name: pomeriumIngressClass,
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: pomeriumControllerName,
		},
	}}, pomeriumControllerName), "our ingress class")

	assert.False(t, isManagingClass(ctx, &networkingv1.Ingress{
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
	}}, pomeriumControllerName), "ignore other ingress classes")

	assert.True(t, isManagingClass(ctx, &networkingv1.Ingress{
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
	}}, pomeriumControllerName), "deprecated method used by http solvers: ingress class in annotation")

	assert.True(t, isManagingClass(ctx, &networkingv1.Ingress{}, []networkingv1.IngressClass{{
		ObjectMeta: metav1.ObjectMeta{
			Name: pomeriumIngressClass,
			Annotations: map[string]string{
				IngressClassDefaultAnnotationKey: "true",
			},
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: pomeriumControllerName,
		},
	}}, pomeriumControllerName), "default ingress")

}

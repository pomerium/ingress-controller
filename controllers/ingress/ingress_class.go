package ingress

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// IngressClassAnnotationKey although deprecated, still may be used by the HTTP solvers even for v1 Ingress resources
	// see https://kubernetes.io/blog/2020/04/02/improvements-to-the-ingress-api-in-kubernetes-1.18/#deprecating-the-ingress-class-annotation
	IngressClassAnnotationKey = "kubernetes.io/ingress.class"

	// IngressClassDefaultAnnotationKey see https://kubernetes.io/docs/concepts/services-networking/ingress/#default-ingress-class
	IngressClassDefaultAnnotationKey = "ingressclass.kubernetes.io/is-default-class"
	// DefaultCertSecretKey is an annotation that may be added to ingressClass
	// nolint:gosec
	DefaultCertSecretKey = "default-cert-secret"
)

type ingressManageResult struct {
	reasonIfNot string
	managed     bool
}

var (
	ingressIsManaged = &ingressManageResult{managed: true}
)

func (r *ingressController) isManaging(ctx context.Context, ing *networkingv1.Ingress) (*ingressManageResult, error) {
	_, err := r.getManagingClass(ctx, ing)
	if err == nil {
		return ingressIsManaged, nil
	}

	if status := apierrors.APIStatus(nil); errors.As(err, &status) {
		return nil, err
	}

	return &ingressManageResult{
		managed:     false,
		reasonIfNot: err.Error(),
	}, nil
}

func (r *ingressController) getManagingClass(ctx context.Context, ing *networkingv1.Ingress) (*networkingv1.IngressClass, error) {
	// if controller is started with explicit list of namespaces to watch,
	// ignore all ingress resources coming from other namespaces
	if len(r.namespaces) > 0 && !r.namespaces[ing.Namespace] {
		return nil, fmt.Errorf("ingress %s/%s is not in the namespace list this controller is managing", ing.Namespace, ing.Name)
	}

	icl := new(networkingv1.IngressClassList)
	if err := r.Client.List(ctx, icl); err != nil {
		return nil, err
	}

	var className string
	if ing.Spec.IngressClassName != nil {
		className = *ing.Spec.IngressClassName
	} else if className = ing.Annotations[IngressClassAnnotationKey]; className != "" {
		log.FromContext(ctx).Info(fmt.Sprintf("use of deprecated annotation %s, please use spec.ingressClassName instead", IngressClassAnnotationKey))
	}

	if className == "" {
		for _, ic := range icl.Items {
			if ic.Spec.Controller != r.controllerName {
				continue
			}
			class := ic
			if isDefault, _ := isDefaultIngressClass(&class); isDefault {
				return &ic, nil
			}
		}
		return nil, fmt.Errorf("the ingress did not specify an ingressClass, and no ingressClass managed by controller %s is marked as default", r.controllerName)
	}

	for _, ic := range icl.Items {
		if ic.Spec.Controller != r.controllerName {
			continue
		}
		if className == ic.Name {
			return &ic, nil
		}
	}

	return nil, fmt.Errorf("IngressClass %s not found or is not assigned to this controller %s", className, r.controllerName)
}

func getAnnotation(dict map[string]string, key string) (string, error) {
	if dict == nil {
		return "", fmt.Errorf("annotation %s is missing", key)
	}
	txt, ok := dict[key]
	if !ok {
		return "", fmt.Errorf("annotation %s is missing", key)
	}
	return txt, nil
}

func namespacedName(name string) (*types.NamespacedName, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return nil, errors.New("should be in namespace/name format")
	}
	return &types.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
}

func isDefaultIngressClass(ic *networkingv1.IngressClass) (bool, error) {
	txt, err := getAnnotation(ic.Annotations, IngressClassDefaultAnnotationKey)
	if err != nil {
		return false, err
	}
	val, err := strconv.ParseBool(txt)
	if err != nil {
		return false, fmt.Errorf("invalid value for annotation %s: %w", IngressClassDefaultAnnotationKey, err)
	}
	return val, nil
}

func getDefaultCertSecretName(ic *networkingv1.IngressClass, prefix string) (*types.NamespacedName, error) {
	txt, err := getAnnotation(ic.Annotations, fmt.Sprintf("%s/%s", prefix, DefaultCertSecretKey))
	if err != nil {
		return nil, err
	}
	return namespacedName(txt)
}

package pomerium

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

func TestBuildIngressConfigValidatesIngressesInIsolation(t *testing.T) {
	ics := []*model.IngressConfig{
		buildConfigTestIngress("app-a", "uid-a"),
		buildConfigTestIngress("broken", "skip"),
		buildConfigTestIngress("app-b", "uid-b"),
	}

	var validatedRouteCounts []int
	validator := func(_ context.Context, cfg *pb.Config, id string) error {
		validatedRouteCounts = append(validatedRouteCounts, len(cfg.Routes))
		if id == "skip" {
			return errors.New("invalid ingress")
		}
		return nil
	}

	cfg := buildIngressConfig(context.Background(), ics, validator)

	assert.Equal(t, []int{1, 1, 1}, validatedRouteCounts,
		"validation input must not grow with the accumulated full-state config")
	require.Len(t, cfg.Routes, 2)
	assert.Contains(t, cfg.Routes[0].Name, "app-a")
	assert.Contains(t, cfg.Routes[1].Name, "app-b")
}

func TestBuildIngressConfigMatchesAccumulatedBuild(t *testing.T) {
	ics := []*model.IngressConfig{
		buildConfigTestIngress("app-a", "uid-a"),
		buildConfigTestIngress("app-b", "uid-b"),
		buildConfigTestIngress("app-c", "uid-c"),
	}
	tlsName := types.NamespacedName{Namespace: "default", Name: "tls"}
	ics[0].Secrets = map[types.NamespacedName]*corev1.Secret{tlsName: {
		ObjectMeta: metav1.ObjectMeta{Namespace: tlsName.Namespace, Name: tlsName.Name},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("certificate"),
			corev1.TLSPrivateKeyKey: []byte("private-key"),
		},
	}}

	legacy := new(pb.Config)
	for _, ic := range ics {
		cfg := proto.Clone(legacy).(*pb.Config)
		require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
		addCerts(cfg, ic.Secrets)
		legacy = cfg
	}
	next := buildIngressConfig(context.Background(), ics, func(context.Context, *pb.Config, string) error {
		return nil
	})
	ensureDeterministicConfigOrder(legacy)
	ensureDeterministicConfigOrder(next)

	assert.True(t, proto.Equal(legacy, next))
}

func buildConfigTestIngress(name string, uid types.UID) *model.IngressConfig {
	const annotationPrefix = "ingress.pomerium.io"
	serviceName := types.NamespacedName{Namespace: "default", Name: "echo"}
	pathType := networkingv1.PathTypePrefix
	return &model.IngressConfig{
		AnnotationPrefix: annotationPrefix,
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      name,
				UID:       uid,
				Annotations: map[string]string{
					fmt.Sprintf("%s/%s", annotationPrefix, model.UseServiceProxy): "true",
				},
			},
			Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
				Host: name + ".example.test",
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path:     "/",
						PathType: &pathType,
						Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
							Name: "echo",
							Port: networkingv1.ServiceBackendPort{Name: "http"},
						}},
					}},
				}},
			}}},
		},
		Services: map[types.NamespacedName]*corev1.Service{serviceName: {
			ObjectMeta: metav1.ObjectMeta{Namespace: serviceName.Namespace, Name: serviceName.Name},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.96.0.10",
				Ports: []corev1.ServicePort{{
					Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080),
				}},
			},
		}},
	}
}

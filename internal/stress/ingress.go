// Package stress provides a set of stress tests for the ingress controller
package stress

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// IngressLoadTestConfig is the configuration for the ingress load test
type IngressLoadTestConfig struct {
	// ReadinessTimeout is the timeout to wait for the ingress to become ready
	ReadinessTimeout time.Duration
	// IngressClass is the ingress controller class
	IngressClass string
	// IngressCount is the number of ingresses to create and mutate
	IngressCount int
	// Domain is the domain to use for the ingresses
	Domain string
	// ServiceName is the name of the service to point the ingresses at
	ServiceName types.NamespacedName
	// ServicePortNames is the list of ports to use on the service, should be more then one
	// so that updates to the ingress can be tested
	ServicePortNames []string
	// Client is kubernetes client
	Client *kubernetes.Clientset
}

// IngressLoadTest is the ingress load test
type IngressLoadTest struct {
	IngressLoadTestConfig

	log       zerolog.Logger
	ingresses []networkingv1.Ingress
}

// Run runs the ingress load test
func (l *IngressLoadTest) Run(ctx context.Context) (err error) {
	l.log = zerolog.Ctx(ctx).With().Str("component", "ingress-load-test").Logger()

	if err := l.cleanup(ctx); err != nil {
		return fmt.Errorf("cleanup before run: %w", err)
	}

	defer func() {
		if err != nil {
			l.log.Error().Err(err).Msg("run failed, cleaning up")
		} else {
			l.log.Info().Msg("cleaning up...")
		}

		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := l.cleanup(cleanupCtx); err != nil {
			l.log.Error().Err(err).Msg("failed to cleanup")
		} else {
			l.log.Info().Msg("clean up completed")
		}
	}()

	if err := l.createIngress(ctx); err != nil {
		return fmt.Errorf("create ingress: %w", err)
	}

	if err := l.waitIngressAvailable(ctx, 0); err != nil {
		return fmt.Errorf("wait ingress available: %w", err)
	}

	i := 0
	for ctx.Err() == nil {
		i++
		l.log.Info().Int("iteration", i).Msg("starting iteration")

		if err := l.updateIngress(ctx, i); err != nil {
			return fmt.Errorf("update ingress: %w", err)
		}
		if err := l.waitIngressAvailable(ctx, i); err != nil {
			return fmt.Errorf("wait ingress available: %w", err)
		}
	}

	return ctx.Err()
}

func (l *IngressLoadTest) getHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func (l *IngressLoadTest) getURLs() []string {
	var urls []string
	for _, ingress := range l.ingresses {
		urls = append(urls, (&url.URL{Scheme: "https", Host: ingress.Spec.Rules[0].Host, Path: "/"}).String())
	}
	return urls
}

func (l *IngressLoadTest) waitIngressAvailable(ctx context.Context, i int) error {
	ctx, cancel := context.WithTimeout(ctx, l.ReadinessTimeout)
	defer cancel()

	httpClient := l.getHTTPClient()

	l.log.Info().Msg("waiting for ingresses to become available")
	now := time.Now()
	err := AwaitReadyMulti(ctx, httpClient, l.getURLs(), getHeaderForIteration(i))
	if err != nil {
		l.log.Err(err).Msg("failed to wait for ingress to become available")
		return fmt.Errorf("failed to wait for ingress to become available: %w", err)
	}
	l.log.Info().Dur("elapsed", time.Since(now)).Msg("ingresses are available")
	return nil
}

func getHeaderForIteration(i int) map[string]string {
	return map[string]string{
		"x-pomerium-stress-test-iteration": fmt.Sprintf("%d", i),
	}
}

func getAnnotationForIteration(i int) map[string]string {
	txt, err := json.Marshal(getHeaderForIteration(i))
	if err != nil {
		panic(err)
	}
	return map[string]string{
		"ingress.pomerium.io/set_response_headers":                string(txt),
		"ingress.pomerium.io/allow_public_unauthenticated_access": "true",
	}
}

func (l *IngressLoadTest) updateIngress(ctx context.Context, i int) error {
	l.log.Info().Msg("updating ingresses")
	now := time.Now()
	// update the ingress by setting the service port to the next one in the list
	// and adding an annotation that updates the response header
	for _, ingress := range l.ingresses {
		obj, err := l.Client.NetworkingV1().Ingresses(ingress.Namespace).Get(ctx, ingress.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get ingress %s: %w", ingress.Name, err)
		}
		obj.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name = l.ServicePortNames[i%len(l.ServicePortNames)]
		obj.Annotations = getAnnotationForIteration(i)
		if _, err := l.Client.NetworkingV1().Ingresses(ingress.Namespace).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update ingress %s: %w", ingress.Name, err)
		}
	}

	l.log.Info().Dur("elapsed", time.Since(now)).Msg("ingresses updated")
	return nil
}

// createIngress creates ingresses
func (l *IngressLoadTest) createIngress(ctx context.Context) error {
	l.log.Info().Msg("creating ingresses...")
	pathType := networkingv1.PathTypePrefix
	for i := 0; i < l.IngressCount; i++ {
		spec := networkingv1.IngressSpec{
			IngressClassName: &l.IngressClass,
			Rules: []networkingv1.IngressRule{
				{
					Host: fmt.Sprintf("ingress-%d.%s", i, l.Domain),
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: l.ServiceName.Name,
											Port: networkingv1.ServiceBackendPort{
												Name: l.ServicePortNames[0],
											}}}}}}}}}}

		annotations := getAnnotationForIteration(0)

		obj, err := l.Client.NetworkingV1().Ingresses(l.ServiceName.Namespace).Create(ctx, &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("ingress-%d", i),
				Labels: map[string]string{
					// default label indicating this ingress was created by this test
					"ingress-stress-test": "true",
				},
				Annotations: annotations,
			},
			Spec: spec,
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ingress %+v: %w", spec, err)
		}
		l.ingresses = append(l.ingresses, *obj)
	}
	l.log.Info().Int("count", len(l.ingresses)).Msg("created ingresses")
	return nil
}

// cleanup deletes all ingresses created by this test based on the labels
func (l *IngressLoadTest) cleanup(ctx context.Context) error {
	l.log.Info().Msg("cleaning up ingresses")

	ingresses, err := l.Client.NetworkingV1().Ingresses(l.ServiceName.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "ingress-stress-test=true",
	})
	if err != nil {
		return fmt.Errorf("failed to list ingresses: %w", err)
	}

	// delete all ingresses
	for _, ingress := range ingresses.Items {
		if err := l.Client.NetworkingV1().Ingresses(l.ServiceName.Namespace).Delete(ctx, ingress.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete ingress %s: %w", ingress.Name, err)
		}
	}

	l.log.Info().Int("count", len(ingresses.Items)).Msg("deleted ingresses")
	return nil
}

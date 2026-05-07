package certificate

import (
	"context"
	"fmt"

	certmanager_v1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ingress_v1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	databrokerpb "github.com/pomerium/pomerium/pkg/grpc/databroker"
)

const (
	managedByLabelName  = "app.kubernetes.io/managed-by"
	managedByLabelValue = "pomerium.io/certificate-controller"
)

type certificateController struct {
	globalSettingsName types.NamespacedName
	kubernetesClient   client.Client
	dataBrokerClient   databrokerpb.DataBrokerServiceClient
}

// NewCertificateController creates a new certificate controller.
func NewCertificateController(
	mgr controllerruntime.Manager,
	globalSettingsName types.NamespacedName,
	dataBrokerClient databrokerpb.DataBrokerServiceClient,
) error {
	c := &certificateController{
		globalSettingsName: globalSettingsName,
		kubernetesClient:   mgr.GetClient(),
		dataBrokerClient:   dataBrokerClient,
	}

	if err := indexField(mgr, "type", func(o *core_v1.Secret) string {
		return string(o.Type)
	}); err != nil {
		return fmt.Errorf("error creating field index: %w", err)
	}

	enqueueRequest := handler.EnqueueRequestsFromMapFunc(
		func(_ context.Context, _ client.Object) []reconcile.Request {
			return []reconcile.Request{{}}
		})

	err := controllerruntime.NewControllerManagedBy(mgr).
		Named("certificate").
		Watches(new(core_v1.Secret), enqueueRequest).
		Watches(new(ingress_v1.Pomerium), enqueueRequest).
		Watches(new(certmanager_v1.Certificate), enqueueRequest).
		Complete(c)
	if err != nil {
		return fmt.Errorf("error building certificate controller: %w", err)
	}

	return nil
}

func (c *certificateController) Reconcile(ctx context.Context, _ controllerruntime.Request) (res controllerruntime.Result, err error) {
	log.FromContext(ctx).Info("certificate-controller: reconciling")

	var cl certmanager_v1.CertificateList
	if err := c.kubernetesClient.List(ctx, &cl,
		client.MatchingLabels{
			managedByLabelName: managedByLabelValue,
		}); err != nil {
		return res, fmt.Errorf("error listing certmanager certificates: %w", err)
	}
	log.FromContext(ctx).Info("certificate-controller", "certificates", cl.Items)

	var settings ingress_v1.Pomerium
	if err := c.kubernetesClient.Get(ctx, c.globalSettingsName, &settings); err != nil {
		return res, fmt.Errorf("error retrieving pomerium settings: %w", err)
	}

	var clusterIssuer string
	if settings.Spec.CertificateAutoProvision != nil && settings.Spec.CertificateAutoProvision.ClusterIssuer != nil {
		clusterIssuer = *settings.Spec.CertificateAutoProvision.ClusterIssuer
	}
	if clusterIssuer == "" {
		log.FromContext(ctx).Info("certificate-controller: automatic certificate provisioning is disabled")

		// remove all existing certificates
		for _, ci := range cl.Items {
			log.FromContext(ctx).Info("certificate-controller: deleting certificate", "name", ci.Name, "namespace", ci.Namespace)
			if err := c.kubernetesClient.Delete(ctx, &ci); err != nil {
				return res, fmt.Errorf("error deleting existing certificate (%s/%s): %w", ci.Namespace, ci.Name, err)
			}
		}

		return res, nil
	}

	var sl core_v1.SecretList
	if err := c.kubernetesClient.List(ctx, &sl,
		client.MatchingFields{
			"type": string(core_v1.SecretTypeTLS),
		}); err != nil {
		return res, fmt.Errorf("error listing secrets: %w", err)
	}
	log.FromContext(ctx).Info("certificate-controller", "secrets", sl.Items)

	return res, nil
}

func indexField[T any, TPtr interface {
	*T
	client.Object
}](mgr controllerruntime.Manager, field string, fn func(obj TPtr) string) error {
	return mgr.GetFieldIndexer().
		IndexField(context.Background(), TPtr(new(T)), field,
			func(o client.Object) []string {
				return []string{fn(any(o).(TPtr))}
			})
}

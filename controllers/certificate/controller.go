package certificate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	certmanager_v1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanager_meta_v1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/hashicorp/go-set/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	core_v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pomerium_ingress_v1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/util"
	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
	databrokerpb "github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"
)

const (
	managedByLabelName       = "app.kubernetes.io/managed-by"
	managedByLabelValue      = "pomerium-certificate-controller"
	dataBrokerConfigRecordID = "pomerium-certificate-controller-config"
)

type certificateController struct {
	cfg              *controllerConfig
	kubernetesClient client.Client
	dataBrokerClient databrokerpb.DataBrokerServiceClient

	dataBrokerCollector *dataBrokerCollector
}

// NewCertificateController creates a new certificate controller.
func NewCertificateController(
	mgr controllerruntime.Manager,
	dataBrokerClient databrokerpb.DataBrokerServiceClient,
	options ...Option,
) {
	c := &certificateController{
		cfg:              getControllerConfig(options...),
		kubernetesClient: mgr.GetClient(),
		dataBrokerClient: dataBrokerClient,
	}
	c.dataBrokerCollector = newDataBrokerCollector(c)
	go c.run(mgr)
}

func (c *certificateController) Reconcile(ctx context.Context, _ controllerruntime.Request) (res controllerruntime.Result, err error) {
	log.FromContext(ctx).Info("certificate-controller: reconciling")
	return res, c.reconcile(ctx)
}

func (c *certificateController) reconcile(ctx context.Context) error {
	// retrieve the settings, certificates and secrets

	var settings pomerium_ingress_v1.Pomerium
	if err := c.kubernetesClient.Get(ctx, c.cfg.globalSettingsName, &settings); err != nil {
		return fmt.Errorf("error retrieving pomerium settings: %w", err)
	}

	var namespace string
	var issuer certmanager_meta_v1.IssuerReference
	if settings.Spec.CertificateAutoProvision != nil && settings.Spec.CertificateAutoProvision.ClusterIssuer != nil {
		issuer = certmanager_meta_v1.IssuerReference{
			Kind: "ClusterIssuer",
			Name: *settings.Spec.CertificateAutoProvision.ClusterIssuer,
		}
	} else if settings.Spec.CertificateAutoProvision != nil && settings.Spec.CertificateAutoProvision.Issuer != nil {
		name, err := util.ParseNamespacedName(*settings.Spec.CertificateAutoProvision.Issuer)
		if err != nil {
			return fmt.Errorf("error parsing certificate auto provision issuer: %w", err)
		}
		namespace = name.Namespace
		issuer = certmanager_meta_v1.IssuerReference{
			Kind: "Issuer",
			Name: name.Name,
		}
	}

	if c.cfg.namespace != nil {
		namespace = *c.cfg.namespace
	}

	// if no namespace was defined, use the current pod namespace
	if namespace == "" {
		var err error
		namespace, err = GetInClusterNamespace()
		if err != nil {
			return err
		}
	}

	var cl certmanager_v1.CertificateList
	if err := c.kubernetesClient.List(ctx, &cl,
		client.InNamespace(namespace),
		client.MatchingLabels{
			managedByLabelName: managedByLabelValue,
		}); err != nil {
		return fmt.Errorf("error listing certmanager certificates: %w", err)
	}

	var sl core_v1.SecretList
	if err := c.kubernetesClient.List(ctx, &sl,
		client.InNamespace(namespace),
		client.MatchingLabels{
			managedByLabelName: managedByLabelValue,
		}); err != nil {
		return fmt.Errorf("error listing secrets: %w", err)
	}

	return errors.Join(
		c.reconcileCertificates(ctx, namespace, issuer, cl.Items),
		c.reconcileSecrets(ctx, sl.Items),
	)
}

func (c *certificateController) reconcileCertificates(
	ctx context.Context,
	namespace string,
	issuer certmanager_meta_v1.IssuerReference,
	certificates []certmanager_v1.Certificate,
) error {
	// delete any certificates with a different cluster issuer
	var certificatesForIssuer []certmanager_v1.Certificate
	for _, cert := range certificates {
		if cert.Spec.IssuerRef.Kind != issuer.Kind || cert.Spec.IssuerRef.Name != issuer.Name {
			if err := c.deleteCertificate(ctx, &cert); err != nil {
				return err
			}
		} else {
			certificatesForIssuer = append(certificatesForIssuer, cert)
		}
	}

	// if there's no cluster issuer, stop the collector and don't provision any certificates
	if issuer.Name == "" {
		c.dataBrokerCollector.Stop()
		return nil
	}

	// determine the missing names
	if err := c.dataBrokerCollector.Sync(); err != nil {
		return fmt.Errorf("error syncing databroker data: %w", err)
	}
	missingNames := set.From(c.dataBrokerCollector.MissingNames())

	// remove any missing names for which we've already created certificates
	for _, cert := range certificatesForIssuer {
		used := false
		for _, name := range cert.Spec.DNSNames {
			if missingNames.Contains(name) {
				missingNames.Remove(name)
				used = true
			}
		}
		// if the certificate is not being used, remove it
		if !used {
			if err := c.deleteCertificate(ctx, &cert); err != nil {
				return err
			}
		}
	}

	// create any certificates for any missing names
	for name := range missingNames.Items() {
		if err := c.createCertificate(ctx, namespace, issuer, name); err != nil {
			return err
		}
	}

	return nil
}

func (c *certificateController) reconcileSecrets(
	ctx context.Context,
	secrets []core_v1.Secret,
) error {
	// make sure secrets are sorted so we get a deterministic encoding
	secrets = slices.SortedFunc(slices.Values(secrets), func(x, y core_v1.Secret) int {
		return cmp.Or(cmp.Compare(x.Name, y.Name), cmp.Compare(x.UID, y.UID))
	})

	cfg := new(configpb.Config)
	for _, s := range secrets {
		certPEM := s.Data["tls.crt"]
		keyPEM := s.Data["tls.key"]
		if len(certPEM) > 0 && len(keyPEM) > 0 {
			if cfg.Settings == nil {
				cfg.Settings = new(configpb.Settings)
			}
			cfg.Settings.Certificates = append(cfg.Settings.Certificates, &configpb.Settings_Certificate{
				CertBytes: certPEM,
				KeyBytes:  keyPEM,
			})
		}
	}

	return c.upsertConfig(ctx, cfg)
}

func (c *certificateController) run(mgr controllerruntime.Manager) {
	ctx := context.Background()

	// wait for all the CRDs to be available
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for _, gvk := range []schema.GroupVersionKind{
		schema.FromAPIVersionAndKind("cert-manager.io/v1", "Certificate"),
		schema.FromAPIVersionAndKind("ingress.pomerium.io/v1", "Pomerium"),
	} {
		for i := 0; ; i++ {
			_, err := mgr.GetCache().GetInformerForKind(ctx, gvk)
			if err == nil {
				break
			}
			if i == 0 {
				log.FromContext(ctx).
					Info("certificate-controller: required CRD does not exist, the certificate controller will not run until it is created",
						"controller", c.cfg.controllerName,
						"group-version-kind", gvk.String())
			}
			<-ticker.C
		}
	}

	err := controllerruntime.NewControllerManagedBy(mgr).
		Named(c.cfg.controllerName).
		Watches(new(core_v1.Secret), &handler.EnqueueRequestForObject{}).
		Watches(new(pomerium_ingress_v1.Pomerium), &handler.EnqueueRequestForObject{}).
		Watches(new(certmanager_v1.Certificate), &handler.EnqueueRequestForObject{}).
		Complete(c)
	if err != nil {
		log.FromContext(ctx).Error(err, "error building certificate controller")
	}
}

func (c *certificateController) createCertificate(
	ctx context.Context,
	namespace string,
	issuer certmanager_meta_v1.IssuerReference,
	dnsName string,
) error {
	k8sName := "pomerium-certificate-" + rand.String(16)
	cert := &certmanager_v1.Certificate{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      k8sName,
			Namespace: namespace,
			Labels: map[string]string{
				managedByLabelName: managedByLabelValue,
			},
		},
		Spec: certmanager_v1.CertificateSpec{
			SecretName: k8sName,
			SecretTemplate: &certmanager_v1.CertificateSecretTemplate{
				Labels: map[string]string{
					managedByLabelName: managedByLabelValue,
				},
			},
			DNSNames:  []string{dnsName},
			IssuerRef: issuer,
		},
	}
	log.FromContext(ctx).Info("certificate-controller: creating certificate",
		"namespace", namespace,
		"issuer-kind", issuer.Kind,
		"issuer-name", issuer.Name,
		"dns-name", dnsName)
	if err := c.kubernetesClient.Create(ctx, cert); err != nil {
		return fmt.Errorf("error creating certificate: %w", err)
	}
	return nil
}

func (c *certificateController) deleteCertificate(ctx context.Context, cert *certmanager_v1.Certificate) error {
	log.FromContext(ctx).Info("certificate-controller: deleting certificate",
		"name", cert.Name,
		"namespace", cert.Namespace)
	if err := c.kubernetesClient.Delete(ctx, cert); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("error deleting existing certificate (%s/%s): %w", cert.Namespace, cert.Name, err)
	}
	// also try and delete the corresponding secret
	_ = c.deleteSecret(ctx, &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: cert.Namespace,
			Name:      cert.Spec.SecretName,
		},
	})
	return nil
}

func (c *certificateController) deleteSecret(ctx context.Context, s *core_v1.Secret) error {
	log.FromContext(ctx).Info("certificate-controller: deleting secret",
		"name", s.Name,
		"namespace", s.Namespace)
	if err := c.kubernetesClient.Delete(ctx, s); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("error deleting existing secret (%s/%s): %w", s.Namespace, s.Name, err)
	}
	return nil
}

// upsertConfig either saves the config to the databroker or if the config is
// empty, deletes the config from the databroker. A "Put" is only done if the
// data changes.
func (c *certificateController) upsertConfig(ctx context.Context, cfg *configpb.Config) error {
	record := &databrokerpb.Record{
		Type: grpcutil.GetTypeURL(cfg),
		Id:   dataBrokerConfigRecordID,
	}
	current, err := c.dataBrokerClient.Get(ctx, &databrokerpb.GetRequest{
		Type: record.Type,
		Id:   record.Id,
	})
	if status.Code(err) == codes.NotFound {
		// if the config is empty and the record doesn't already exist, there's nothing to do
		if proto.Equal(cfg, new(configpb.Config)) {
			return nil
		}
	} else if err != nil {
		return fmt.Errorf("error getting current config: %w", err)
	} else {
		record = current.Record
	}

	// if the config is empty, delete the record
	if proto.Equal(cfg, new(configpb.Config)) {
		record.DeletedAt = timestamppb.Now()
		log.FromContext(ctx).V(1).Info("certificate-controller: deleting config",
			"record-type", record.Type,
			"record-id", record.Id)
		_, err = c.dataBrokerClient.Put(ctx, &databrokerpb.PutRequest{
			Records: []*databrokerpb.Record{record},
		})
		if err != nil {
			return fmt.Errorf("error deleting config: %w", err)
		}
		return nil
	}

	// compare the new data to the existing data
	data, err := anypb.New(cfg)
	if err != nil {
		return fmt.Errorf("error creating config data: %w", err)
	}
	if proto.Equal(data, record.Data) {
		// nothing to do, the data is the same
		return nil
	}
	record.Data = data

	// save the config
	log.FromContext(ctx).Info("certificate-controller: saving config",
		"record-type", record.Type,
		"record-id", record.Id,
		"certificates-count", len(cfg.GetSettings().GetCertificates()))
	_, err = c.dataBrokerClient.Put(ctx, &databrokerpb.PutRequest{
		Records: []*databrokerpb.Record{record},
	})
	if err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}

	return nil
}

var inClusterNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// GetInClusterNamespace returns the namespace of the pod the process is
// running in, as exposed by the kubernetes service account token volume.
func GetInClusterNamespace() (string, error) {
	raw, err := os.ReadFile(inClusterNamespaceFile)
	if err != nil {
		return "", fmt.Errorf("error reading in-cluster namespace: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

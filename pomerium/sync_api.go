package pomerium

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/gosimple/slug"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/sdk-go"
	"github.com/pomerium/sdk-go/proto/pomerium"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium/gateway"
)

func NewUnifiedAPIReconciler(
	url, token string,
) Reconciler {
	client := sdk.NewClient(
		sdk.WithURL(url),
		sdk.WithAPIToken(token))
	return &APIReconciler{
		apiClient:  client,
		secretsMap: model.NewTLSSecretsMap(),
	}
}

var (
	_ = IngressReconciler((*APIReconciler)(nil))
	_ = GatewayReconciler((*APIReconciler)(nil))
	_ = ConfigReconciler((*APIReconciler)(nil))
)

// APIReconciler updates pomerium configuration using the unified API.
type APIReconciler struct {
	apiClient sdk.Client
	k8sClient client.Client

	secretsMap *model.TLSSecretsMap
}

const (
	apiRouteIDAnnotationPrefix = "api.pomerium.io/route-id-"
	apiPolicyIDAnnotation      = "api.pomerium.io/policy-id"
	apiKeyPairIDAnnotation     = "api.pomerium.io/keypair-id"
	apiFinalizer               = "api.pomerium.io/finalizer"
)

var originatorID = "ingress-controller"

func (r *APIReconciler) SetK8sClient(client client.Client) {
	r.k8sClient = client
}

func (r *APIReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	var anyChanges bool

	// Sync any referenced TLS secrets to API keypairs.
	var tlsSecrets []*corev1.Secret
	for _, s := range ic.Secrets {
		if s.Type == corev1.SecretTypeTLS {
			tlsSecrets = append(tlsSecrets, s)
		}
	}
	changed, err := r.upsertKeyPairs(ctx, tlsSecrets)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || changed

	changed, err = r.upsertOneIngress(ctx, ic)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || changed

	// Remove keypairs corresponding to any newly-unreferenced TLS secrets.
	unreferencedSecrets := r.secretsMap.UpdateIngress(ic)
	anyDeletes, err := r.deleteKeyPairs(ctx, unreferencedSecrets...)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || anyDeletes

	return anyChanges, nil
}

func (r *APIReconciler) Set(ctx context.Context, ics []*model.IngressConfig) (bool, error) {
	tlsSecrets := make(map[types.NamespacedName]*corev1.Secret)
	for _, ic := range ics {
		// Collect all the referenced TLS secrets. These need to be synced
		// before the routes, so that a route can reference a keypair ID.
		for n, s := range ic.Secrets {
			if s.Type == corev1.SecretTypeTLS {
				r.secretsMap.Add(model.KeyForObject(ic), n)
				tlsSecrets[n] = s
			}
		}
	}

	anyChanges, err := r.upsertKeyPairs(ctx, slices.Collect(maps.Values(tlsSecrets)))
	if err != nil {
		return anyChanges, err
	}

	// TODO: should we do an initial scan here for any Secrets that were deleted
	// but still have our finalizer attached?

	var errs []error
	for _, ic := range ics {
		changed, err := r.upsertOneIngress(ctx, ic)
		if err != nil {
			errs = append(errs, err)
		}
		anyChanges = anyChanges || changed
	}

	return anyChanges, errors.Join(errs...)
}

func (r *APIReconciler) upsertOneIngress(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	logger := log.FromContext(ctx).WithName("APIReconciler.upsertOneIngress")

	routes, err := ingressToRoutes(ctx, ic)
	if err != nil {
		return false, fmt.Errorf("couldn't convert ingress to routes: %w", err)
	}

	var anyChanges bool

	originalIngress := ic.Ingress.DeepCopy()
	defer func() {
		if !anyChanges {
			return
		}
		err := r.k8sClient.Patch(ctx, ic.Ingress, client.MergeFrom(originalIngress))
		if err != nil {
			logger.Error(err, "couldn't patch ingress", "ingress", ic.Name)
		}
	}()

	anyChanges = anyChanges || controllerutil.AddFinalizer(ic.Ingress, apiFinalizer)

	kv, err := removeKeyPrefix(ic.Ingress.Annotations, ic.AnnotationPrefix)

	changed, updatedPolicyID, err := r.syncIngressPolicy(ctx, ic.Ingress, kv)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || changed

	var policyIDs []string
	if updatedPolicyID != "" {
		policyIDs = []string{updatedPolicyID}
	}

	var keypairErrs []error
	keyPairIDForAnnotation := func(annotation string) *string {
		secretName, hasAnnotation := kv.TLS[annotation]
		if !hasAnnotation {
			return nil
		}
		secret := ic.Secrets[types.NamespacedName{Namespace: ic.Namespace, Name: secretName}]
		if secret == nil {
			keypairErrs = append(keypairErrs,
				fmt.Errorf("internal error - secret %q referenced by ingress %q not fetched",
					secretName, ic.GetIngressNamespacedName()))
			return nil
		}
		keyPairID := secret.Annotations[apiKeyPairIDAnnotation]
		if keyPairID == "" {
			keypairErrs = append(keypairErrs,
				fmt.Errorf("internal error - secret %q referenced by ingress %q missing keypair ID",
					secretName, ic.GetIngressNamespacedName()))
			return nil
		}
		return &keyPairID
	}
	tlsCustomCAKeyPairID := keyPairIDForAnnotation(model.TLSCustomCASecret)
	tlsClientKeyPairID := keyPairIDForAnnotation(model.TLSClientSecret)
	tlsDownstreamClientCAKeyPairID := keyPairIDForAnnotation(model.TLSDownstreamClientCASecret)
	if len(keypairErrs) > 0 {
		return anyChanges, errors.Join(keypairErrs...)
	}

	unusedRouteIDAnnotations := allRouteIDAnnotations(ic.Annotations)

	for i, route := range routes {
		k := routeIDAnnotationForIndex(i)
		delete(unusedRouteIDAnnotations, k)

		// Swap out any inline policies for the policy ID reference, and swap
		// out any TLS secrets for keypair ID references.
		route.Policies = nil
		route.PolicyIds = policyIDs
		route.TlsCustomCa = ""
		route.TlsCustomCaKeyPairId = tlsCustomCAKeyPairID
		route.TlsClientCert = ""
		route.TlsClientKey = ""
		route.TlsClientKeyPairId = tlsClientKeyPairID
		route.TlsDownstreamClientCa = ""
		route.TlsDownstreamClientCaKeyPairId = tlsDownstreamClientCAKeyPairID

		// Clear the route StatName as it can't currently be set in Pomerium Zero.
		route.StatName = nil

		changed, err := r.upsertOneRoute(ctx, ic.Annotations[k], route)
		if err != nil {
			return anyChanges, err
		}
		if ic.Annotations[k] == "" {
			setAnnotation(ic, k, *route.Id)
		}
		anyChanges = anyChanges || changed
	}

	// If the Ingress object has any other route ID annotations, these indicate
	// API routes that are no longer in use and should be deleted. (This will be
	// the case when an existing Rule is deleted from an Ingress.)
	anyDeletes, err := r.deleteRoutes(ctx, ic.Ingress, unusedRouteIDAnnotations)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || anyDeletes

	// If there was a linked policy that is no no longer needed, delete it.
	// (This cannot be done until all of the linked routes are updated to no
	// longer reference the existing policy ID.)
	if ic.Annotations[apiPolicyIDAnnotation] != "" && updatedPolicyID == "" {
		deleted, err := r.deletePolicy(ctx, ic.Ingress)
		if err != nil {
			return anyChanges, err
		}
		anyChanges = anyChanges || deleted
	}

	return anyChanges, nil
}

func (r *APIReconciler) upsertKeyPairs(
	ctx context.Context,
	secrets []*corev1.Secret,
) (bool, error) {
	logger := log.FromContext(ctx).WithName("APIReconciler.upsertKeyPairs")
	logger.Info("syncing...", "count", len(secrets))

	var anyChanges bool
	for _, secret := range secrets {
		changed, err := r.upsertOneKeyPair(ctx, secret)
		if err != nil {
			return anyChanges, err
		}
		anyChanges = anyChanges || changed
	}
	return anyChanges, nil
}

func (r *APIReconciler) upsertOneKeyPair(
	ctx context.Context,
	secret *corev1.Secret,
) (bool, error) {
	logger := log.FromContext(ctx).WithName("APIReconciler.upsertOneKeyPair")

	cert, hasTLSCert := secret.Data[corev1.TLSCertKey]
	if !hasTLSCert {
		cert = secret.Data[model.CAKey]
	}

	name := slug.Make(fmt.Sprintf("%s %s", secret.Namespace, secret.Name))
	keyPair := &pomerium.KeyPair{
		Name:         &name,
		Certificate:  cert,
		Key:          secret.Data[corev1.TLSPrivateKeyKey],
		OriginatorId: &originatorID,
	}

	existingKeyPairID := secret.Annotations[apiKeyPairIDAnnotation]
	if existingKeyPairID == "" {
		logger.Info("creating new keypair...", "name", name)

		// No linked keypair, so we need to create one.
		updatedKeyPairID, err := r.createKeyPair(ctx, keyPair)
		if err != nil {
			return false, fmt.Errorf("couldn't create key pair: %w", err)
		}

		originalSecret := secret.DeepCopy()
		setAnnotation(secret, apiKeyPairIDAnnotation, updatedKeyPairID)
		controllerutil.AddFinalizer(secret, apiFinalizer)
		err = r.k8sClient.Patch(ctx, secret, client.MergeFrom(originalSecret))
		if err != nil {
			logger := log.FromContext(ctx).WithName("APIReconciler.upsertOneKeyPair")
			logger.Error(err, "couldn't patch secret", "name", secret.Name)
		}
		return true, err
	}

	logger.Info("found existing keypair...", "name", name)

	keyPair.Id = &existingKeyPairID
	return r.upsertKeyPair(ctx, keyPair)
}

// SetConfig updates just the shared config settings
func (r *APIReconciler) SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error) {
	// Remove keypairs corresponding to any newly-unreferenced TLS secrets.
	unreferencedSecrets := r.secretsMap.UpdateConfig(cfg)
	anyDeletes, err := r.deleteKeyPairs(ctx, unreferencedSecrets...)
	if err != nil {
		return changes, err
	}
	changes = changes || anyDeletes

	// Upsert current TLS certificates (server certs + CA certs).
	allCertSecrets := make([]*corev1.Secret, 0, len(cfg.CASecrets)+len(cfg.Certs))
	allCertSecrets = append(allCertSecrets, cfg.CASecrets...)
	allCertSecrets = append(allCertSecrets, slices.Collect(maps.Values(cfg.Certs))...)
	changedKeyPair, err := r.upsertKeyPairs(ctx, allCertSecrets)
	if err != nil {
		return changes, err
	}
	changes = changes || changedKeyPair

	pbConfig := new(pb.Config)
	if err = ApplyConfig(ctx, pbConfig, cfg); err != nil {
		return false, fmt.Errorf("couldn't convert settings: %w", err)
	}
	// Remove any inline certificates (certificates are already synced above).
	pbConfig.Settings.Certificates = nil
	pbConfig.Settings.CertificateAuthority = nil

	settings, err := convertProto[*pomerium.Settings](pbConfig.Settings)
	if err != nil {
		return false, err
	}

	resp, err := r.apiClient.GetSettings(ctx, connect.NewRequest(&pomerium.GetSettingsRequest{}))
	if err != nil {
		return false, err
	}

	// Mask any settings that cannot be set via the Pomerium CRD
	existing := resp.Msg.Settings
	existing.Address = nil
	existing.Autocert = nil
	existing.ClusterId = nil
	existing.GrpcAddress = nil
	existing.GrpcInsecure = nil
	existing.Id = nil
	existing.InsecureServer = nil
	existing.NamespaceId = nil
	existing.SharedSecret = nil

	// Mask timestamp metadata.
	existing.CreatedAt = nil
	existing.ModifiedAt = nil

	if proto.Equal(existing, settings) {
		// No changes needed.
		return changes, nil
	}

	logger := log.FromContext(ctx).WithName("APIReconciler.SetConfig")
	logger.V(1).Info("needs settings update", "diff", cmp.Diff(existing, settings, protocmp.Transform()))

	_, err = r.apiClient.UpdateSettings(ctx, connect.NewRequest(&pomerium.UpdateSettingsRequest{
		Settings: settings,
	}))
	changes = changes || (err == nil)
	return changes, err
}

// Delete removes pomerium routes corresponding to this ingress.
func (r *APIReconciler) Delete(ctx context.Context, name types.NamespacedName, ingress *networkingv1.Ingress) (bool, error) {
	if ingress == nil {
		return false, nil
	}

	originalIngress := ingress.DeepCopy()
	defer func() {
		err := r.k8sClient.Patch(ctx, ingress, client.MergeFrom(originalIngress))
		if err != nil {
			logger := log.FromContext(ctx).WithName("APIReconciler.Delete")
			logger.Error(err, "couldn't patch ingress", "name", ingress.Name)
		}
	}()

	routeAnnotations := allRouteIDAnnotations(ingress.Annotations)
	anyDeletes, err := r.deleteRoutes(ctx, ingress, routeAnnotations)
	if err != nil {
		return anyDeletes, err
	}

	policyDeleted, err := r.deletePolicy(ctx, ingress)
	if err != nil {
		return anyDeletes, err
	}
	anyDeletes = anyDeletes || policyDeleted

	// Remove keypairs corresponding to any newly-unreferenced TLS secrets.
	unreferencedSecrets := r.secretsMap.RemoveEntity(model.Key{
		Kind:           ingress.Kind,
		NamespacedName: name,
	})
	anyKeyPairDeletes, err := r.deleteKeyPairs(ctx, unreferencedSecrets...)
	if err != nil {
		return anyDeletes, err
	}
	anyDeletes = anyDeletes || anyKeyPairDeletes

	controllerutil.RemoveFinalizer(ingress, apiFinalizer)

	return anyDeletes, nil
}

// SetGatewayConfig applies Gateway-defined configuration.
func (r *APIReconciler) SetGatewayConfig(
	ctx context.Context,
	gatewayConfig *model.GatewayConfig,
) (changes bool, err error) {
	// Sync keypairs.
	unreferencedSecrets := r.secretsMap.UpdateGatewayConfig(gatewayConfig)
	anyDeletes, err := r.deleteKeyPairs(ctx, unreferencedSecrets...)
	if err != nil {
		return changes, err
	}
	changes = changes || anyDeletes

	changedKeyPair, err := r.upsertKeyPairs(ctx, gatewayConfig.Certificates)
	if err != nil {
		return changes, err
	}
	changes = changes || changedKeyPair

	// Extract and sync any policies.
	changedPolicy, policyIDs, err := r.syncGatewayPolicies(ctx, gatewayConfig)
	if err != nil {
		return changes, err
	}
	changes = changes || changedPolicy

	for i := range gatewayConfig.Routes {
		gr := &gatewayConfig.Routes[i]
		originalRoute := gr.HTTPRoute.DeepCopy()

		if gr.DeletionTimestamp == nil {
			routes := gateway.TranslateRoutes(ctx, gatewayConfig, gr)
			for i, route := range routes {
				// Replace any inline policy with a policy ID reference.
				if err := replaceInlinePolicies(route, policyIDs); err != nil {
					return changes, err
				}

				k := routeIDAnnotationForIndex(i)
				routeChanged, err := r.upsertOneRoute(ctx, gr.Annotations[k], route)
				if err != nil {
					return changes, err
				}
				changes = changes || routeChanged
				if gr.Annotations[k] == "" {
					setAnnotation(gr, k, *route.Id)
				}
			}
			controllerutil.AddFinalizer(gr, apiFinalizer)
		} else {
			// This HTTPRoute was deleted, so delete any synced Pomerium routes.
			anyDeletes, err := r.deleteRoutes(ctx, gr, allRouteIDAnnotations(gr.Annotations))
			if err != nil {
				return changes, err
			}
			changes = changes || anyDeletes

			controllerutil.RemoveFinalizer(gr, apiFinalizer)
		}

		if err := r.k8sClient.Patch(ctx, gr.HTTPRoute, client.MergeFrom(originalRoute)); err != nil {
			logger := log.FromContext(ctx).WithName("APIReconciler.SetGatewayConfig")
			logger.Error(err, "couldn't patch httproute", "name", gr.Name)
			return changes, err
		}
	}

	removed, err := r.removeDeletedGatewayPolicies(ctx, gatewayConfig)
	if err != nil {
		return changes, err
	}
	changes = changes || removed

	return changes, nil
}

func (r *APIReconciler) syncGatewayPolicies(
	ctx context.Context, gatewayConfig *model.GatewayConfig,
) (changes bool, policyIDs map[string]string, err error) {
	policyIDs = map[string]string{}
	for _, ef := range gatewayConfig.ExtensionFilters {
		pf, isPolicyFilter := ef.(*gateway.PolicyFilter)
		if !isPolicyFilter {
			continue
		}

		obj := pf.GetObject()
		if obj.DeletionTimestamp != nil {
			// Note: if this policy was assigned to any routes we cannot delete
			// it yet.
			continue
		}
		originalObj := obj.DeepCopy()

		var route pb.Route
		pf.ApplyToRoute(&route)
		policy, err := convertProto[*pomerium.Policy](route.Policies[0])
		if err != nil {
			return changes, nil, err
		}
		policy.OriginatorId = &originatorID
		policy.Rego = nil
		policyName := slug.Make(fmt.Sprintf("%s %s", obj.Namespace, obj.Name))
		policy.Name = &policyName
		if id := obj.Annotations[apiPolicyIDAnnotation]; id != "" {
			policy.Id = &id
		}

		changedPolicy, err := r.upsertPolicy(ctx, policy)
		if err != nil {
			return changes, nil, err
		} else if changedPolicy {
			changes = true
			setAnnotation(obj, apiPolicyIDAnnotation, policy.GetId())
			controllerutil.AddFinalizer(obj, apiFinalizer)
		}

		policyIDs[policy.GetSourcePpl()] = policy.GetId()

		if err := r.k8sClient.Patch(ctx, obj, client.MergeFrom(originalObj)); err != nil {
			logger := log.FromContext(ctx).WithName("APIReconciler.syncGatewayPolicies")
			logger.Error(err, "couldn't patch policyfilter", "name", obj.Name)
			return changes, nil, err
		}
	}

	return changes, policyIDs, nil
}

func (r *APIReconciler) removeDeletedGatewayPolicies(
	ctx context.Context, gatewayConfig *model.GatewayConfig,
) (changes bool, err error) {
	for _, ef := range gatewayConfig.ExtensionFilters {
		pf, isPolicyFilter := ef.(*gateway.PolicyFilter)
		if !isPolicyFilter {
			continue
		}

		obj := pf.GetObject()
		if obj.DeletionTimestamp == nil {
			continue
		}

		deleted, err := r.deletePolicy(ctx, obj)
		if err != nil {
			return changes, err
		}
		changes = changes || deleted

		originalObj := obj.DeepCopy()
		controllerutil.RemoveFinalizer(obj, apiFinalizer)
		if err := r.k8sClient.Patch(ctx, obj, client.MergeFrom(originalObj)); err != nil {
			logger := log.FromContext(ctx).WithName("APIReconciler.removeDeletedGatewayPolicies")
			logger.Error(err, "couldn't patch policyfilter", "name", obj.Name)
			return changes, err
		}
	}
	return changes, nil
}

// replaceInlinePolicies translates from inline route policies to policy IDs
// from the policyIDs map (keyed by source PPL).
func replaceInlinePolicies(route *pb.Route, policyIDs map[string]string) error {
	for i, p := range route.Policies {
		if p.SourcePpl == nil {
			return fmt.Errorf("internal error - source PPL missing from policy %d of route %q", i, route.GetName())
		}
		id, ok := policyIDs[*p.SourcePpl]
		if !ok {
			return fmt.Errorf("internal error - policy ID not found for policy %d of route %q", i, route.GetName())
		}
		route.PolicyIds = append(route.PolicyIds, id)
	}
	route.Policies = nil
	return nil
}

func (r *APIReconciler) upsertOneRoute(ctx context.Context, id string, route *pb.Route) (bool, error) {
	logger := log.FromContext(ctx).WithName("APIReconciler.upsertOneRoute")

	apiRoute, err := convertProto[*pomerium.Route](route)
	if err != nil {
		return false, err
	}
	apiRoute.OriginatorId = &originatorID

	var existing *pomerium.Route
	if id != "" {
		resp, err := r.apiClient.GetRoute(ctx, connect.NewRequest(&pomerium.GetRouteRequest{
			Id: id,
		}))
		if err != nil && connect.CodeOf(err) == connect.CodeNotFound {
			return false, err
		} else if err == nil {
			existing = resp.Msg.Route
		}

		apiRoute.Id = new(id)
	} else {
		// The ID must be assigned during route creation. (We can't use the
		// derived ID from the conversion logic.)
		apiRoute.Id = nil
	}

	if existing == nil {
		apiRoute.OriginatorId = &originatorID

		// If the route does not currently exist, create it.
		resp, err := r.apiClient.CreateRoute(ctx, connect.NewRequest(&pomerium.CreateRouteRequest{
			Route: apiRoute,
		}))
		if err != nil {
			return false, err
		}
		route.Id = resp.Msg.Route.Id
		return true, nil
	}

	// Clear the fields that should be ignored when looking for changes.
	existing.NamespaceId = nil
	existing.CreatedAt = nil
	existing.ModifiedAt = nil
	existing.AssignedPolicies = nil
	existing.EnforcedPolicies = nil
	existing.StatName = nil

	if proto.Equal(existing, apiRoute) {
		// No changes needed.
		return false, nil
	}

	logger.V(1).Info("updating existing route",
		"id", apiRoute.GetId(),
		"diff", cmp.Diff(existing, apiRoute, protocmp.Transform()))

	_, err = r.apiClient.UpdateRoute(ctx, connect.NewRequest(&pomerium.UpdateRouteRequest{
		Route: apiRoute,
	}))
	return err == nil, err
}

// deleteRoutes deletes routes corresponding to the keys in annotationKeys and
// removes the corresponding annotations from obj. Returns true if any routes
// were deleted, and an error if some delete operation failed.
func (r *APIReconciler) deleteRoutes(
	ctx context.Context, obj client.Object, annotationKeys map[string]struct{},
) (bool, error) {
	var anyDeletes bool
	annotations := obj.GetAnnotations()
	for k := range annotationKeys {
		_, err := r.apiClient.DeleteRoute(ctx, connect.NewRequest(&pomerium.DeleteRouteRequest{
			Id: annotations[k],
		}))
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			return anyDeletes, err
		}
		delete(annotations, k)
		anyDeletes = true
	}
	return anyDeletes, nil
}

func (r *APIReconciler) syncIngressPolicy(
	ctx context.Context, ingress *networkingv1.Ingress, kv *keys,
) (changed bool, updatedPolicyID string, err error) {
	existingPolicyID := ingress.Annotations[apiPolicyIDAnnotation]
	name := slug.Make(fmt.Sprintf("%s %s policy", ingress.Namespace, ingress.Name))
	policy, err := keysToPolicy(kv, name)
	if err != nil {
		return false, "", fmt.Errorf("couldn't extract ingress policy: %w", err)
	}

	if policy == nil {
		// No policy needed.
		return false, "", nil
	}

	apiPolicy, err := convertProto[*pomerium.Policy](policy)
	if err != nil {
		return false, "", fmt.Errorf("internal error: %w", err)
	}
	apiPolicy.OriginatorId = &originatorID
	if existingPolicyID != "" {
		apiPolicy.Id = &existingPolicyID
	}

	// Create or update the Pomerium policy as needed.
	changed, err = r.upsertPolicy(ctx, apiPolicy)
	if err != nil {
		return false, "", fmt.Errorf("couldn't update ingress policy: %w", err)
	}
	updatedPolicyID = apiPolicy.GetId()
	setAnnotation(ingress, apiPolicyIDAnnotation, updatedPolicyID)
	return changed, updatedPolicyID, nil
}

func (r *APIReconciler) createPolicy(ctx context.Context, policy *pomerium.Policy) (newID string, err error) {
	resp, err := r.apiClient.CreatePolicy(ctx, connect.NewRequest(&pomerium.CreatePolicyRequest{
		Policy: policy,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.GetPolicy().GetId(), nil
}

// upsertPolicy will create or update a Pomerium policy. If a new ID is
// assigned, policy.Id will be updated.
func (r *APIReconciler) upsertPolicy(ctx context.Context, policy *pomerium.Policy) (changed bool, err error) {
	var existing *pomerium.Policy
	if id := policy.GetId(); id != "" {
		resp, err := r.apiClient.GetPolicy(ctx, connect.NewRequest(&pomerium.GetPolicyRequest{
			Id: id,
		}))
		if err == nil {
			existing = resp.Msg.Policy
		} else if connect.CodeOf(err) != connect.CodeNotFound {
			return false, err
		}
	}

	// If there is no existing policy, create one.
	if existing == nil {
		resp, err := r.apiClient.CreatePolicy(ctx, connect.NewRequest(&pomerium.CreatePolicyRequest{
			Policy: policy,
		}))
		if err != nil {
			return false, err
		}
		policy.Id = resp.Msg.Policy.Id
		return true, nil
	}

	// Zero out fields that should be ignored when looking for changes.
	existing.NamespaceId = nil
	existing.CreatedAt = nil
	existing.ModifiedAt = nil
	existing.AssignedRoutes = nil
	existing.Enforced = falseToNil(existing.Enforced)

	if proto.Equal(existing, policy) {
		// No changes needed.
		return false, nil
	}

	logger := log.FromContext(ctx).WithName("APIReconciler.upsertPolicy")
	logger.V(1).Info("updating existing policy", "id", policy.GetId(), "diff", cmp.Diff(existing, policy, protocmp.Transform()))

	_, err = r.apiClient.UpdatePolicy(ctx, connect.NewRequest(&pomerium.UpdatePolicyRequest{
		Policy: policy,
	}))
	return err == nil, err
}

// deletePolicy deletes the policy for the ingress and clears its policy ID
// annotation. Returns true if any changes were made, or an error if the delete
// operation failed.
func (r *APIReconciler) deletePolicy(
	ctx context.Context, obj client.Object,
) (deleted bool, err error) {
	annotations := obj.GetAnnotations()
	policyID := annotations[apiPolicyIDAnnotation]
	if policyID == "" {
		return false, nil
	}
	_, err = r.apiClient.DeletePolicy(ctx, connect.NewRequest(&pomerium.DeletePolicyRequest{
		Id: policyID,
	}))
	if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
		return false, err
	}
	delete(annotations, apiPolicyIDAnnotation)
	return true, nil
}

func (r *APIReconciler) createKeyPair(ctx context.Context, keyPair *pomerium.KeyPair) (newID string, err error) {
	resp, err := r.apiClient.CreateKeyPair(ctx, connect.NewRequest(&pomerium.CreateKeyPairRequest{
		KeyPair: keyPair,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.GetKeyPair().GetId(), nil
}

func (r *APIReconciler) upsertKeyPair(ctx context.Context, keyPair *pomerium.KeyPair) (changed bool, err error) {
	resp, err := r.apiClient.GetKeyPair(ctx, connect.NewRequest(&pomerium.GetKeyPairRequest{
		Id: keyPair.GetId(),
	}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			// If the existing key pair was deleted, recreate it.
			_, err := r.createKeyPair(ctx, keyPair)
			return err == nil, err
		}
		return false, err
	}

	// Zero out fields that should be ignored when looking for changes.
	existing := resp.Msg.KeyPair
	existing.NamespaceId = nil
	existing.CreatedAt = nil
	existing.ModifiedAt = nil
	existing.CertificateInfo = nil
	existing.Origin = pomerium.KeyPairOrigin_KEY_PAIR_ORIGIN_UNKNOWN
	existing.Status = pomerium.KeyPairStatus_KEY_PAIR_STATUS_UNKNOWN

	if proto.Equal(existing, keyPair) {
		// No changes needed.
		return false, nil
	}

	logger := log.FromContext(ctx).WithName("APIReconciler.upsertKeyPair")
	logger.V(1).Info("updating existing keypair",
		"id", keyPair.GetId(),
		"diff", cmp.Diff(existing, keyPair, protocmp.Transform()))

	_, err = r.apiClient.UpdateKeyPair(ctx, connect.NewRequest(&pomerium.UpdateKeyPairRequest{
		KeyPair: keyPair,
	}))
	return err == nil, err
}

// deleteKeyPairs deletes the keypairs corresponding to the given Secret names,
// clearing the keypair ID annotation for each. Returns true if any deletes were
// successful, or an error if some delete operation failed.
func (r *APIReconciler) deleteKeyPairs(
	ctx context.Context, secretNames ...types.NamespacedName,
) (bool, error) {
	logger := log.FromContext(ctx).WithName("APIReconciler.deleteKeyPairs")
	var anyDeletes bool
	for _, n := range secretNames {
		secret := new(corev1.Secret)
		err := r.k8sClient.Get(ctx, n, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// If we can't retrieve this Secret, we won't know the ID of the
				// keypair to delete. There's not much we can do in this case.
				logger.Info("could not delete keypair (secret not found)", "secret", n)
				continue
			}
			return anyDeletes, err
		}
		keyPairID := secret.Annotations[apiKeyPairIDAnnotation]
		if keyPairID == "" {
			continue
		}
		_, err = r.apiClient.DeleteKeyPair(ctx, connect.NewRequest(&pomerium.DeleteKeyPairRequest{
			Id: keyPairID,
		}))
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			return anyDeletes, err
		}
		originalSecret := secret.DeepCopy()
		delete(secret.Annotations, apiKeyPairIDAnnotation)
		controllerutil.RemoveFinalizer(secret, apiFinalizer)
		if err := r.k8sClient.Patch(ctx, secret, client.MergeFrom(originalSecret)); err != nil {
			logger.Error(err, "couldn't patch secret", "name", secret.Name)
			return anyDeletes, err
		}
		anyDeletes = true
	}
	return anyDeletes, nil
}

func routeIDAnnotationForIndex(i int) string {
	return apiRouteIDAnnotationPrefix + strconv.Itoa(i)
}

func allRouteIDAnnotations(annotations map[string]string) map[string]struct{} {
	m := make(map[string]struct{})
	for k := range annotations {
		if strings.HasPrefix(k, apiRouteIDAnnotationPrefix) {
			m[k] = struct{}{}
		}
	}
	return m
}

func convertProto[Dst, Src proto.Message](msg Src) (Dst, error) {
	// TODO: figure out a way to avoid this extra marshal/unmarshal step
	var newMsg Dst
	b, err := proto.Marshal(msg)
	if err != nil {
		return newMsg, err
	}
	newMsg = newMsg.ProtoReflect().Type().New().Interface().(Dst)
	err = proto.Unmarshal(b, newMsg)
	return newMsg, err
}

func setAnnotation(object client.Object, key, value string) {
	m := object.GetAnnotations()
	if m == nil {
		m = make(map[string]string)
	}
	m[key] = value
	object.SetAnnotations(m)
}

func falseToNil(x *bool) *bool {
	if x != nil && !*x {
		return nil
	}
	return x
}

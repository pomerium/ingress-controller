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

// XXX: this is quick hack for prototyping
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
	// XXX: should probably re-scan all secrets here, looking for any deleted ones?
	tlsSecrets := make(map[types.NamespacedName]*corev1.Secret)
	for _, ic := range ics {
		// Collect all the referenced TLS secrets. These need to be synced
		// before the routes, as a route may reference a keypair by ID.
		for n, s := range ic.Secrets {
			if s.Type == corev1.SecretTypeTLS {
				r.secretsMap.Add(model.KeyForObject(ic), n) // XXX: consider just updateIngress() instead?
				tlsSecrets[n] = s
			}
		}
	}

	anyChanges, err := r.upsertKeyPairs(ctx, slices.Collect(maps.Values(tlsSecrets)))
	if err != nil {
		return anyChanges, err
	}

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
			// XXX: review error handling for other Patch() calls, make sure we log consistently
			logger.Error(err, "patch", "ingress", ic.Name)
		}
	}()

	anyChanges = anyChanges || controllerutil.AddFinalizer(ic.Ingress, apiFinalizer)

	kv, err := removeKeyPrefix(ic.Ingress.Annotations, ic.AnnotationPrefix)

	changed, updatedPolicyID, err := r.syncPolicy(ctx, ic.Ingress, kv)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || changed

	var policyIDs []string
	if updatedPolicyID != "" {
		policyIDs = []string{updatedPolicyID}
	}

	keyPairIDForAnnotation := func(annotation string) *string {
		secretName, hasAnnotation := kv.TLS[annotation]
		if !hasAnnotation {
			return nil
		}
		secret := ic.Secrets[types.NamespacedName{Namespace: ic.Namespace, Name: secretName}]
		if secret == nil {
			// XXX: promote this to an error?
			logger.Info("secret not fetched (internal error)", "annotation", annotation, "secretName", secretName)
		}
		keyPairID := secret.Annotations[apiKeyPairIDAnnotation]
		if keyPairID == "" {
			logger.Info("missing keypair ID annotation (internal error)", "annotation", annotation, "secretName", secretName)
		}
		return &keyPairID
	}
	tlsCustomCAKeyPairID := keyPairIDForAnnotation(model.TLSCustomCASecret)
	tlsClientKeyPairID := keyPairIDForAnnotation(model.TLSClientSecret)
	tlsDownstreamClientCAKeyPairID := keyPairIDForAnnotation(model.TLSDownstreamClientCASecret)

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
		return err == nil, err
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
		// XXX: track "dirty" state?
		err := r.k8sClient.Patch(ctx, ingress, client.MergeFrom(originalIngress))
		if err != nil {
			logger := log.FromContext(ctx).WithName("APIReconciler.upsertOneIngress")
			logger.Error(err, "patch", "ingress", ingress.Name)
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

	for i := range gatewayConfig.Routes {
		gr := &gatewayConfig.Routes[i]
		routes := gateway.TranslateRoutes(ctx, gatewayConfig, gr)
		for i, route := range routes {
			k := routeIDAnnotationForIndex(i)
			routeChanged, err := r.upsertOneRoute(ctx, gr.Annotations[k], route)
			if err != nil {
				return changes, err
			} else if routeChanged {
				changes = true
			}
			if gr.Annotations[k] == "" {
				setAnnotation(gr, k, *route.Id)
			}
		}
	}

	return changes, nil
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

		apiRoute.Id = &id // XXX: new(id) ?
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

	// Zero out fields that should be ignored when looking for changes.
	existing.NamespaceId = nil
	existing.CreatedAt = nil
	existing.ModifiedAt = nil
	existing.AssignedPolicies = nil
	existing.EnforcedPolicies = nil

	// XXX: figure out what to do with stat_name -- it doesn't seem to be preserved by Zero

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

// deleteRoutes deletes routes corresponding to the annotation keys in
// routeAnnotations and removes the corresponding annotations from ingress.
// Returns true if any routes were deleted, and an error if some delete
// operation failed.
func (r *APIReconciler) deleteRoutes(
	ctx context.Context, ingress *networkingv1.Ingress, routeAnnotations map[string]struct{},
) (bool, error) {
	var anyDeletes bool
	for k := range routeAnnotations {
		_, err := r.apiClient.DeleteRoute(ctx, connect.NewRequest(&pomerium.DeleteRouteRequest{
			Id: ingress.Annotations[k],
		}))
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			return anyDeletes, err
		}
		delete(ingress.Annotations, k)
		anyDeletes = true
	}
	return anyDeletes, nil
}

type policyCleanupFn func(context.Context) (bool, error)

func (r *APIReconciler) syncPolicy(
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

	if existingPolicyID == "" {
		// No linked policy, so we need to create one.
		updatedPolicyID, err = r.createPolicy(ctx, apiPolicy)
		if err != nil {
			return false, "", fmt.Errorf("couldn't create ingress policy: %w", err)
		}
		setAnnotation(ingress, apiPolicyIDAnnotation, updatedPolicyID)
		return true, updatedPolicyID, nil
	}

	// Sync any changes to linked policy.
	apiPolicy.Id = &existingPolicyID
	changed, err = r.upsertPolicy(ctx, apiPolicy)
	if err != nil {
		return false, "", fmt.Errorf("couldn't update ingress policy: %w", err)
	}
	return changed, *apiPolicy.Id, nil
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

func (r *APIReconciler) upsertPolicy(ctx context.Context, policy *pomerium.Policy) (changed bool, err error) {
	resp, err := r.apiClient.GetPolicy(ctx, connect.NewRequest(&pomerium.GetPolicyRequest{
		Id: policy.GetId(),
	}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			// If the existing policy was deleted, recreate it.
			_, err := r.createPolicy(ctx, policy)
			return err == nil, err
		}
		return false, err
	}

	// Zero out fields that should be ignored when looking for changes
	existing := resp.Msg.Policy
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
	ctx context.Context, ingress *networkingv1.Ingress,
) (deleted bool, err error) {
	policyID := ingress.Annotations[apiPolicyIDAnnotation]
	if policyID == "" {
		return false, nil
	}
	_, err = r.apiClient.DeletePolicy(ctx, connect.NewRequest(&pomerium.DeletePolicyRequest{
		Id: policyID,
	}))
	if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
		return false, err
	}
	delete(ingress.Annotations, apiPolicyIDAnnotation)
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

	logger := log.FromContext(ctx).WithName("APIReconciler.upsertPolicy")
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

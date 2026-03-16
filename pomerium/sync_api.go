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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/google/go-cmp/cmp"
	"github.com/gosimple/slug"
	"github.com/pomerium/pomerium/pkg/cryptutil"
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
		secretsMap: newSecretsMap(),
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

	// XXX: not currently populated -- need to figure out if we need this or not
	k8sClient client.Client

	secretsMap *secretsMap
}

const (
	apiRouteIDAnnotationPrefix = "api.pomerium.io/route-id-"
	apiPolicyIDAnnotation      = "api.pomerium.io/policy-id"
	apiKeypairIDAnnotation     = "api.pomerium.io/keypair-id"
	apiFinalizer               = "api.pomerium.io/finalizer"
)

var originatorID = "ingress-controller"

// XXX: the bool return value from this method is maybe unused in practice? can we remove it?
func (r *APIReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	var anyChanges bool

	changed, err := r.upsertOneIngress(ctx, ic)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || changed

	// Look for any changes to TLS secrets
	unreferencedSecrets := r.secretsMap.updateIngress(ic)
	anyDeletes, err := r.deleteKeyPairs(ctx, unreferencedSecrets...)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || anyDeletes

	return r.Set(ctx, []*model.IngressConfig{ic})
}

func (r *APIReconciler) Set(ctx context.Context, ics []*model.IngressConfig) (bool, error) {
	r.secretsMap.reset()
	secrets := make(map[types.NamespacedName]*corev1.Secret)
	var anyChanges bool
	var errs []error
	for _, ic := range ics {
		changed, err := r.upsertOneIngress(ctx, ic)
		if err != nil {
			errs = append(errs, err)
		}
		anyChanges = anyChanges || changed

		// Keep track of all the referenced TLS secrets.
		for n, s := range ic.Secrets {
			if s.Type == corev1.SecretTypeTLS {
				r.secretsMap.add(ic.GetIngressNamespacedName(), n)
				secrets[n] = s
			}
		}
	}

	r.upsertCerts(ctx, slices.Collect(maps.Values(secrets)))

	return anyChanges, errors.Join(errs...)
}

func (r *APIReconciler) upsertOneIngress(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	routes, err := ingressToRoutes(ctx, ic)
	if err != nil {
		return false, fmt.Errorf("couldn't convert ingress to routes: %w", err)
	}

	var anyChanges bool

	anyChanges = anyChanges || controllerutil.AddFinalizer(ic.Ingress, apiFinalizer)

	changed, updatedPolicyID, err := r.syncPolicy(ctx, ic)
	if err != nil {
		return anyChanges, err
	}
	anyChanges = anyChanges || changed

	var policyIDs []string
	if updatedPolicyID != "" {
		policyIDs = []string{updatedPolicyID}
	}

	unusedRouteIDAnnotations := allRouteIDAnnotations(ic.Annotations)

	for i, route := range routes {
		k := routeIDAnnotationForIndex(i)
		delete(unusedRouteIDAnnotations, k)

		// Swap out any inline policies for the policy ID reference.
		route.Policies = nil
		route.PolicyIds = policyIDs

		changed, err := r.upsertOneRoute(ctx, ic.Annotations[k], route)
		if err != nil {
			return anyChanges, err
		}
		if ic.Annotations[k] == "" {
			ic.Annotations[k] = *route.Id
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

// XXX: rename to syncKeyPairs?
func (r *APIReconciler) upsertCerts(
	ctx context.Context,
	secrets []*corev1.Secret,
) (bool, error) {
	var anyChanges bool
	for _, secret := range secrets {
		changed, err := r.upsertOneCert(ctx, secret)
		if err != nil {
			return anyChanges, err
		}
		anyChanges = anyChanges || changed
	}
	return anyChanges, nil
}

func (r *APIReconciler) upsertOneCert(
	ctx context.Context,
	secret *corev1.Secret,
) (bool, error) {
	name := slug.Make(fmt.Sprintf("%s %s", secret.Namespace, secret.Name))
	keyPair := &pomerium.KeyPair{
		Name:         &name,
		Certificate:  secret.Data[corev1.TLSCertKey],
		Key:          secret.Data[corev1.TLSPrivateKeyKey],
		OriginatorId: &originatorID,
	}

	existingKeyPairID := secret.Annotations[apiKeypairIDAnnotation]
	if existingKeyPairID == "" {
		// No linked policy, so we need to create one.
		updatedKeyPairID, err := r.createKeyPair(ctx, keyPair)
		if err != nil {
			return false, fmt.Errorf("couldn't create key pair: %w", err)
		}
		secret.Annotations[apiPolicyIDAnnotation] = updatedKeyPairID
		return true, nil
	}

	keyPair.Id = &existingKeyPairID
	return r.upsertKeyPair(ctx, keyPair)
}

// SetConfig updates just the shared config settings
func (r *APIReconciler) SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error) {
	// XXX: TODO
	return false, nil
}

// Delete should delete pomerium routes corresponding to this ingress name
func (r *APIReconciler) Delete(ctx context.Context, _ types.NamespacedName, ingress *networkingv1.Ingress) (bool, error) {
	if ingress == nil {
		return false, nil
	}

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

	controllerutil.RemoveFinalizer(ingress, apiFinalizer)

	return anyDeletes, nil
}

// SetGatewayConfig applies Gateway-defined configuration.
func (r *APIReconciler) SetGatewayConfig(
	ctx context.Context,
	gatewayConfig *model.GatewayConfig,
) (changes bool, err error) {
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
				gr.Annotations[k] = *route.Id
			}
		}
	}

	// XXX: apply settings + certs

	return changes, nil
}

func extractCerts(secrets map[types.NamespacedName]*corev1.Secret) []*pb.Settings_Certificate {
	var certs []*pb.Settings_Certificate
	for _, secret := range secrets {
		if secret.Type != corev1.SecretTypeTLS {
			continue
		}
		certs = append(certs, &pb.Settings_Certificate{
			CertBytes: secret.Data[corev1.TLSCertKey],
			KeyBytes:  secret.Data[corev1.TLSPrivateKeyKey],
		})
	}
	return certs
}

func deriveCertID(cert *pb.Settings_Certificate) {
	// XXX: error handling
	cryptutil.ParsePEMCertificate(cert.CertBytes)

}

//type ConnectMethod[Req, Resp proto.Message] func(context.Context, *connect.Request[Req]) (*connect.Response[Resp], error)

//type entityUpserter[]

func (r *APIReconciler) upsertOneRoute(ctx context.Context, id string, route *pb.Route) (bool, error) {
	apiRoute, err := convertProto[*pomerium.Route](route)
	if err != nil {
		return false, err
	}
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

	// XXX: is it possible for resp.Msg to be nil at this point?
	// XXX: do we need to ignore certain fields in this comparison?
	if proto.Equal(existing, apiRoute) {
		// No changes needed.
		return false, nil
	}

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
	ctx context.Context, ic *model.IngressConfig,
) (changed bool, updatedPolicyID string, err error) {
	existingPolicyID := ic.Annotations[apiPolicyIDAnnotation]
	policy, err := ingressToPolicy(ic)
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
		ic.Annotations[apiPolicyIDAnnotation] = updatedPolicyID
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
		if connect.CodeOf(err) != connect.CodeNotFound {
			// If the existing policy was deleted, recreate it.
			_, err := r.createPolicy(ctx, policy)
			return err == nil, err
		}
		return false, err
	}

	// Zero out fields that should be ignored when looking for changes
	existing := resp.Msg.Policy
	existing.AssignedRoutes = nil
	existing.CreatedAt = nil
	existing.NamespaceId = nil
	existing.ModifiedAt = nil
	if !existing.GetEnforced() {
		existing.Enforced = nil
	}

	if proto.Equal(existing, policy) {
		// No changes needed.
		return false, nil
	}

	// XXX: debugging
	logger := log.FromContext(ctx).WithName("APIReconciler.upsertPolicy")
	logger.Info("updating existing policy", "id", policy.GetId(), "diff", cmp.Diff(existing, policy, protocmp.Transform()))

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
) (bool, error) {
	policyID := ingress.Annotations[apiPolicyIDAnnotation]
	if policyID == "" {
		return false, nil
	}
	_, err := r.apiClient.DeletePolicy(ctx, connect.NewRequest(&pomerium.DeletePolicyRequest{
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
		if connect.CodeOf(err) != connect.CodeNotFound {
			// If the existing key pair was deleted, recreate it.
			_, err := r.createKeyPair(ctx, keyPair)
			return err == nil, err
		}
		return false, err
	}

	// Zero out fields that should be ignored when looking for changes
	existing := resp.Msg.KeyPair
	existing.CreatedAt = nil
	existing.NamespaceId = nil
	existing.ModifiedAt = nil

	if proto.Equal(existing, keyPair) {
		// No changes needed.
		return false, nil
	}

	// XXX: debugging
	logger := log.FromContext(ctx).WithName("APIReconciler.upsertPolicy")
	logger.Info("updating existing policy", "id", keyPair.GetId(), "diff", cmp.Diff(existing, keyPair, protocmp.Transform()))

	_, err = r.apiClient.UpdateKeyPair(ctx, connect.NewRequest(&pomerium.UpdateKeyPairRequest{
		KeyPair: keyPair,
	}))
	return err == nil, err
}

// deletePolicy deletes the policy for the ingress and clears its policy ID
// annotation. Returns true if any changes were made, or an error if the delete
// operation failed.
func (r *APIReconciler) deleteKeyPairs(
	ctx context.Context, secrets ...*corev1.Secret,
) (bool, error) {
	var anyDeletes bool
	for _, s := range secrets {
		keyPairID := s.Annotations[apiKeypairIDAnnotation] // XXX: standardize capitalization
		if keyPairID == "" {
			continue
		}
		_, err := r.apiClient.DeleteKeyPair(ctx, connect.NewRequest(&pomerium.DeleteKeyPairRequest{
			Id: keyPairID,
		}))
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			return anyDeletes, err
		}
		delete(s.Annotations, apiKeypairIDAnnotation)
		return true, nil
	}
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
	// XXX: figure out a way to use the connect client without this extra marshaling
	var newMsg Dst
	b, err := proto.Marshal(msg)
	if err != nil {
		return newMsg, err
	}
	newMsg = newMsg.ProtoReflect().Type().New().Interface().(Dst)
	err = proto.Unmarshal(b, newMsg)
	return newMsg, err
}

type secretsMap struct {
	// Mapping from Ingress to names to TLS secret names. This is to track
	// when an Ingress is modified in a way that removes a TLS secret.
	ingressToSecrets map[types.NamespacedName]map[types.NamespacedName]struct{}

	// Reverse mapping from TLS secret names to Ingress names. This is to track
	// when a TLS secret is no longer in use by any Ingress.
	secretToIngresses map[types.NamespacedName]map[types.NamespacedName]struct{}
}

func newSecretsMap() *secretsMap {
	return &secretsMap{
		ingressToSecrets:  make(map[types.NamespacedName]map[types.NamespacedName]struct{}),
		secretToIngresses: make(map[types.NamespacedName]map[types.NamespacedName]struct{}),
	}
}

func (m *secretsMap) reset() {
	clear(m.ingressToSecrets)
	clear(m.secretToIngresses)
}

func (m *secretsMap) updateIngress(ic *model.IngressConfig) []types.NamespacedName {
	currentSecrets := make(map[types.NamespacedName]struct{})
	for n, s := range ic.Secrets {
		if s.Type == corev1.SecretTypeTLS {
			currentSecrets[n] = struct{}{}
		}
	}

	n := ic.GetIngressNamespacedName()
	previousSecrets := m.ingressToSecrets[n]
	m.ingressToSecrets[n] = currentSecrets

	for s := range currentSecrets {
		delete(previousSecrets, s)
	}

	var unreferencedSecrets []types.NamespacedName
	for s := range previousSecrets {
		delete(m.secretToIngresses[s], n)
		if len(m.secretToIngresses[s]) == 0 {
			unreferencedSecrets = append(unreferencedSecrets, s)
		}
	}
	return unreferencedSecrets
}

func (m *secretsMap) add(ingress, secret types.NamespacedName) {
	ensureMapEntry(m.ingressToSecrets, ingress)
	m.ingressToSecrets[ingress][secret] = struct{}{}
	ensureMapEntry(m.secretToIngresses, secret)
	m.secretToIngresses[secret][ingress] = struct{}{}
}

func (m *secretsMap) remove(ingress, secret types.NamespacedName) {
	delete(m.ingressToSecrets[ingress], secret)
	delete(m.secretToIngresses[secret], ingress)
}

func ensureMapEntry(m map[types.NamespacedName]map[types.NamespacedName]struct{}, k types.NamespacedName) {
	if _, exists := m[k]; !exists {
		m[k] = make(map[types.NamespacedName]struct{})
	}
}

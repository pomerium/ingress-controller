package pomerium

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
	return &APIReconciler{apiClient: client}
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
}

const (
	apiRouteIDAnnotationPrefix = "api.pomerium.io/route-id-"
	apiFinalizer               = "api.pomerium.io/finalizer"
)

var originatorID = "ingress-controller"

// XXX: the bool return value from this method is maybe unused in practice? can we remove it?
func (r *APIReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	return r.Set(ctx, []*model.IngressConfig{ic})
}

func (r *APIReconciler) Set(ctx context.Context, ics []*model.IngressConfig) (bool, error) {
	secrets := make(map[types.NamespacedName]*corev1.Secret)
	var anyChanges bool
	var errs []error
	for _, ic := range ics {
		changed, err := r.upsertOneIngress(ctx, ic)
		if err != nil {
			errs = append(errs, err)
		}
		anyChanges = anyChanges || changed

		// Collect all of the referenced secrets into one map.
		maps.Copy(secrets, ic.Secrets)
	}

	certs := extractCerts(secrets)
	r.upsertCerts(ctx, certs)

	return anyChanges, errors.Join(errs...)
}

func (r *APIReconciler) upsertOneIngress(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	routes, err := ingressToRoutes(ctx, ic)
	if err != nil {
		return false, fmt.Errorf("couldn't convert ingress to routes: %w", err)
	}

	controllerutil.AddFinalizer(ic, apiFinalizer)

	unusedRouteIDAnnotations := allRouteIDAnnotations(ic.Annotations)

	var anyChanges bool
	for i, route := range routes {
		k := routeIDAnnotationForIndex(i)
		delete(unusedRouteIDAnnotations, k)
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
	r.deleteRoutes(ctx, ic.Ingress, unusedRouteIDAnnotations)

	return anyChanges, nil
}

func (r *APIReconciler) upsertCerts(
	ctx context.Context,
	certs []*pb.Settings_Certificate,
) (bool, error) {
	// XXX: TODO
	return false, nil
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

func (r *APIReconciler) upsertOneCert(ctx context.Context, cert *pb.Settings_Certificate) (bool, error) {
	apiCert, err := convertProto[*pomerium.Settings_Certificate](cert)
	if err != nil {
		return false, err
	}

	resp, err := r.apiClient.GetKeyPair(ctx, connect.NewRequest(&pomerium.GetKeyPairRequest{
		Id: cert.GetId(),
	}))
	notFound := connect.CodeOf(err) == connect.CodeNotFound
	if err != nil && !notFound {
		return false, err
	}

	if notFound {
		// If the route does not currently exist, create it.
		_, err := r.apiClient.CreateKeyPair(ctx, connect.NewRequest(&pomerium.CreateKeyPairRequest{
			//Route: apiCert,
		}))
		return err == nil, err
	}

	// XXX: is it possible for resp.Msg to be nil at this point?
	// XXX: do we need to ignore certain fields in this comparison?
	if proto.Equal(resp.Msg, apiCert) {
		// No changes needed.
		return false, nil
	}

	_, err = r.apiClient.UpdateKeyPair(ctx, connect.NewRequest(&pomerium.UpdateKeyPairRequest{
		//Route: apiCert,
	}))
	return err == nil, err
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

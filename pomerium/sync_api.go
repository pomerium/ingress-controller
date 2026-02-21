package pomerium

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/pomerium/pkg/cryptutil"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/sdk-go"
	"github.com/pomerium/sdk-go/proto/pomerium"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium/gateway"
)

var (
	_ = IngressReconciler((*APIReconciler)(nil))
	_ = GatewayReconciler((*APIReconciler)(nil))
	_ = ConfigReconciler((*APIReconciler)(nil))
)

// APIReconciler updates pomerium configuration using the unified API.
type APIReconciler struct {
	client sdk.Client
}

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

	var anyChanges bool
	for _, route := range routes {
		changed, err := r.upsertOneRoute(ctx, route)
		if err != nil {
			return anyChanges, err
		}
		anyChanges = anyChanges || changed
	}
	return anyChanges, nil
}

func (r *APIReconciler) upsertCerts(
	ctx context.Context,
	certs []*pb.Settings_Certificate,
) (bool, error) {
}

// SetConfig updates just the shared config settings
func (r *APIReconciler) SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error) {
	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get config: %w", err)
	}
	next := new(pb.Config)

	if err = ApplyConfig(ctx, next, cfg); err != nil {
		return false, fmt.Errorf("settings: %w", err)
	}

	return r.saveConfig(ctx, prev, next, r.ConfigID)
}

// Delete should delete pomerium routes corresponding to this ingress name
func (r *APIReconciler) Delete(ctx context.Context, namespacedName types.NamespacedName) (bool, error) {
	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get pomerium config: %w", err)
	}
	cfg := proto.Clone(prev).(*pb.Config)
	if err := deleteRoutes(cfg, namespacedName); err != nil {
		return false, fmt.Errorf("deleting pomerium config records %s: %w", namespacedName.String(), err)
	}
	changed, err := r.saveConfig(ctx, prev, cfg, fmt.Sprintf("%s-%s", namespacedName.Namespace, namespacedName.Name))
	if err != nil {
		return false, fmt.Errorf("updating pomerium config: %w", err)
	}
	return changed, nil
}

// SetGatewayConfig applies Gateway-defined configuration.
func (r *APIReconciler) SetGatewayConfig(
	ctx context.Context,
	config *model.GatewayConfig,
) (changes bool, err error) {
	var routes []*pb.Route
	for i := range config.Routes {
		r := &config.Routes[i]
		routes = append(routes, gateway.TranslateRoutes(ctx, config, r)...)
	}
	for _, route := range routes {
		routeChanged, err := r.upsertOneRoute(ctx, route)
		if err != nil {
			return changes, err
		} else if routeChanged {
			changes = true
		}
	}
	// XXX: apply settings + certs

	return changes, nil
}

// DeleteAll cleans pomerium configuration entirely
func (r *APIReconciler) DeleteAll(ctx context.Context) error {
	// XXX: need to tag all entities created by this controller, and then delete all by this tag
	return fmt.Errorf("not implemented")
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

func (r *APIReconciler) upsertOneRoute(ctx context.Context, route *pb.Route) (bool, error) {
	apiRoute, err := convertProto[*pomerium.Route](route)
	if err != nil {
		return false, err
	}

	resp, err := r.client.GetRoute(ctx, connect.NewRequest(&pomerium.GetRouteRequest{
		Id: route.GetId(),
	}))
	notFound := connect.CodeOf(err) == connect.CodeNotFound
	if err != nil && !notFound {
		return false, err
	}

	if notFound {
		// If the route does not currently exist, create it.
		_, err := r.client.CreateRoute(ctx, connect.NewRequest(&pomerium.CreateRouteRequest{
			Route: apiRoute,
		}))
		return err == nil, err
	}

	// XXX: is it possible for resp.Msg to be nil at this point?
	// XXX: do we need to ignore certain fields in this comparison?
	if proto.Equal(resp.Msg, apiRoute) {
		// No changes needed.
		return false, nil
	}

	_, err = r.client.UpdateRoute(ctx, connect.NewRequest(&pomerium.UpdateRouteRequest{
		Route: apiRoute,
	}))
	return err == nil, err
}

func (r *APIReconciler) upsertOneCert(ctx context.Context, cert *pb.Settings_Certificate) (bool, error) {
	apiCert, err := convertProto[*pomerium.Settings_Certificate](cert)
	if err != nil {
		return false, err
	}

	resp, err := r.client.GetKeyPair(ctx, connect.NewRequest(&pomerium.GetKeyPairRequest{
		Id: cert.GetId(),
	}))
	notFound := connect.CodeOf(err) == connect.CodeNotFound
	if err != nil && !notFound {
		return false, err
	}

	if notFound {
		// If the route does not currently exist, create it.
		_, err := r.client.CreateRoute(ctx, connect.NewRequest(&pomerium.CreateRouteRequest{
			Route: apiCert,
		}))
		return err == nil, err
	}

	// XXX: is it possible for resp.Msg to be nil at this point?
	// XXX: do we need to ignore certain fields in this comparison?
	if proto.Equal(resp.Msg, apiCert) {
		// No changes needed.
		return false, nil
	}

	_, err = r.client.UpdateRoute(ctx, connect.NewRequest(&pomerium.UpdateRouteRequest{
		Route: apiCert,
	}))
	return err == nil, err
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

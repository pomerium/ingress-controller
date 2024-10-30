package gateway

import (
	context "context"
	"fmt"
	golog "log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/hashicorp/go-set/v3"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
)

type gatewayController struct {
	// Client is k8s api server client
	client.Client
	// PomeriumReconciler updates Pomerium service configuration
	pomerium.GatewayReconciler
	controllerName string
	// Registry is used to keep track of dependencies between objects
	//model.Registry
	// XXX: I think we'll need something similar, but aware of the Gateway-specific objects
	// MultiPomeriumStatusReporter is used to report when settings are updated
	//reporter.MultiPomeriumStatusReporter
}

const ControllerName = "pomerium.io/gateway-controller"

// NewGatewayController creates and registers a new controller for Gateway objects.
func NewGatewayController(
	mgr ctrl.Manager,
	pgr pomerium.GatewayReconciler,
	controllerName string,
) error {
	gtc := &gatewayController{
		Client:            mgr.GetClient(),
		GatewayReconciler: pgr,
		controllerName:    controllerName,
	}

	// All updates will trigger the same reconcile request.
	enqueueRequest := handler.EnqueueRequestsFromMapFunc(
		func(_ context.Context, _ client.Object) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Name: ControllerName,
				},
			}}
		})

	err := ctrl.NewControllerManagedBy(mgr).
		Named("gateway").
		Watches(
			&gateway_v1.Gateway{},
			enqueueRequest,
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(
			&gateway_v1.HTTPRoute{},
			enqueueRequest,
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// XXX: make this more efficient (ignore non-referenced secrets?)
		Watches(&corev1.Secret{}, enqueueRequest).
		Complete(gtc)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	return nil
}

func (c *gatewayController) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	// XXX: where does this log output go?
	logger := log.FromContext(ctx).V(1)
	logger.Info("[Gateway] reconciling... ")

	golog.Println(" *** [Gateway] Reconcile *** ") // XXX

	// Fetch all relevant API objects.
	var gcl gateway_v1.GatewayClassList
	var gl gateway_v1.GatewayList
	var hrl gateway_v1.HTTPRouteList
	var sl corev1.SecretList
	if err := c.List(ctx, &gcl); err != nil {
		return ctrl.Result{}, err
	}
	if err := c.List(ctx, &gl); err != nil {
		return ctrl.Result{}, err
	}
	if err := c.List(ctx, &hrl); err != nil {
		return ctrl.Result{}, err
	}
	if err := c.List(ctx, &sl); err != nil {
		return ctrl.Result{}, err
	}
	// XXX: retries if any of those List calls fails?

	// Filter the hierarchy of Gateway objects to just the ones corresponding to this controller.
	// XXX: should we use some ListOptions in the List call to help with filtering?

	golog.Printf("HTTPRoutes: %v", hrl.Items) // XXX

	gcNames := set.New[string](0)
	for i := range gcl.Items {
		gc := &gcl.Items[i]
		if gc.Spec.ControllerName == gateway_v1.GatewayController(c.controllerName) {
			gcNames.Insert(gc.Name)
		}
	}

	var gateways []*gateway_v1.Gateway
	routesByParentRef := make(map[refKey][]*gateway_v1.HTTPRoute)

	for i := range gl.Items {
		g := &gl.Items[i]
		if gcNames.Contains(string(g.Spec.GatewayClassName)) {
			gateways = append(gateways, g)
		}
	}
	for i := range hrl.Items {
		hr := &hrl.Items[i]
		for j := range hr.Spec.ParentRefs {
			pr := &hr.Spec.ParentRefs[j]
			key := refKeyForParentRef(hr, pr)
			routesByParentRef[key] = append(routesByParentRef[key], hr)
		}
	}

	for k, r := range routesByParentRef {
		golog.Printf("%v: %d routes", k, len(r))
	}

	var config model.GatewayConfig
	certificateRefs := set.New[refKey](0)

	// XXX: match routes to gateways and compute hostname intersection

	for _, g := range gateways {
		routes := routesByParentRef[refKeyForObject(g)]
		c.reconcileGateway(ctx, g, routes)

		golog.Printf("Gateway: %s", g.Name) // XXX
		golog.Printf("refKey: %v", refKeyForObject(g))
		golog.Printf("Routes: %v", routes) // XXX

		gatewayHostnames := set.New[gateway_v1.Hostname](0)

		for i := range g.Spec.Listeners {
			l := &g.Spec.Listeners[i]
			if l.Hostname != nil {
				gatewayHostnames.Insert(*l.Hostname)
			}
			if l.TLS != nil {
				// Collect all certificate references.
				for j := range l.TLS.CertificateRefs {
					certificateRefs.Insert(refKeyForCertificateRef(g, &l.TLS.CertificateRefs[j]))
				}
			}
		}

		for _, r := range routes {
			config.Routes = append(config.Routes, model.GatewayHTTPRouteConfig{
				HTTPRoute: r,
				Hostnames: routeHostnames(gatewayHostnames, r.Spec.Hostnames),
			})
		}
	}

	// Collect all referenced certificate Secrets.
	for i := range sl.Items {
		s := &sl.Items[i]
		if certificateRefs.Contains(refKeyForObject(s)) {
			config.Certificates = append(config.Certificates, s)
		}
	}

	c.GatewaySetConfig(ctx, &config)

	return ctrl.Result{}, nil
}

func routeHostnames(
	gatewayHostnames *set.Set[gateway_v1.Hostname],
	routeHostnames []gateway_v1.Hostname,
) []gateway_v1.Hostname {
	// XXX: this also needs to take sectionName into account
	if gatewayHostnames.Empty() {
		return routeHostnames
	}
	if len(routeHostnames) == 0 {
		return gatewayHostnames.Slice()
	}
	return gatewayHostnames.Intersect(set.From(routeHostnames)).Slice()
}

// refKey contains the essential fields of a ParentReference in a form suitable for use as a map key.
type refKey struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func refKeyForParentRef(obj client.Object, ref *gateway_v1.ParentReference) refKey {
	// Group defaults to the Gateway group name.
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.ParentReference.
	group := gateway_v1.GroupName
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	// Kind appears to have a default value but I don't see this clearly spelled out in the API
	// reference. I think Gateway is the only Kind we care about in practice.
	kind := "Gateway"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	// The namespace of a parentRef defaults to the object's own namespace.
	namespace := obj.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return refKey{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      string(ref.Name),
	}
}

func refKeyForCertificateRef(obj client.Object, ref *gateway_v1.SecretObjectReference) refKey {
	group := gateway_v1.GroupName
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := "Secret"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	namespace := obj.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return refKey{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      string(ref.Name),
	}
}

func refKeyForObject(obj client.Object) refKey {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return refKey{
		Group:     gvk.Group,
		Kind:      gvk.Kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func (c *gatewayController) reconcileGateway(
	ctx context.Context,
	gateway *gateway_v1.Gateway,
	httpRoutes []*gateway_v1.HTTPRoute,
) error {
	golog.Printf(" *** reconcileGateway: %s ***", gateway.Name) // XXX

	// TODO: enforce permissions model

	var updateStatus bool

	// TODO: update Gateway status (Addresses, Listeners, Conditions)
	if setListenersStatus(gateway, httpRoutes) {
		updateStatus = true
	}

	if setGatewayConditions(gateway,
		metav1.Condition{
			Type:    string(gateway_v1.GatewayConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gateway_v1.GatewayReasonAccepted),
			Message: "", // XXX
		},
		metav1.Condition{
			Type:    string(gateway_v1.GatewayConditionProgrammed),
			Status:  metav1.ConditionTrue,
			Reason:  string(gateway_v1.GatewayReasonProgrammed),
			Message: "", // XXX
		},
	) {
		updateStatus = true
	}

	if updateStatus {
		if err := c.Status().Update(ctx, gateway); err != nil {
			golog.Printf("couldn't update status for %q: %v", gateway.Name, err) // XXX
			return err
		}
	}

	golog.Printf("gateway %q status: %v", gateway.Name, gateway.Status) // XXX

	// TODO: resolve route references, update status, translate to config

	// XXX: proper support for sectionName >>>
	// ParentRefs must be distinct. This means either that:
	// They select different objects. If this is the case, then parentRef entries are distinct. In terms of fields, this means that the multi-part key defined by group, kind, namespace, and name must be unique across all parentRef entries in the Route.
	// They do not select different objects, but for each optional field used, each ParentRef that selects the same object must set the same set of optional fields to different values. If one ParentRef sets a combination of optional fields, all must set the same combination.
	// Some examples:
	// If one ParentRef sets sectionName, all ParentRefs referencing the same object must also set sectionName.

	return nil
}
func setGatewayConditions(g *gateway_v1.Gateway, conditions ...metav1.Condition) (modified bool) {
	for _, c := range conditions {
		if setGatewayCondition(g, c) {
			modified = true
		}
	}
	return modified
}

func setGatewayCondition(g *gateway_v1.Gateway, condition metav1.Condition) (modified bool) {
	condition.ObservedGeneration = g.Generation
	condition.LastTransitionTime = metav1.Now()

	conds := g.Status.Conditions
	for i := range conds {
		if conds[i].Type == condition.Type {
			// Existing condition found.
			// XXX: does this need to compare the observed generation too?
			if conds[i].Status == condition.Status &&
				conds[i].Reason == condition.Reason &&
				conds[i].Message == condition.Message {
				return false
			}
			conds[i] = condition
			return true
		}
	}
	// No existing condition found, so add it.
	g.Status.Conditions = append(g.Status.Conditions, condition)
	return true
}

// XXX: move to another file?
func upsertConditions(
	conditions *[]metav1.Condition,
	observedGeneration int64,
	condition ...metav1.Condition,
) (modified bool) {
	for _, c := range condition {
		if upsertCondition(conditions, observedGeneration, c) {
			modified = true
		}
	}
	return modified
}

func upsertCondition(
	conditions *[]metav1.Condition,
	observedGeneration int64,
	condition metav1.Condition,
) (modified bool) {
	condition.ObservedGeneration = observedGeneration
	condition.LastTransitionTime = metav1.Now()

	conds := *conditions
	for i := range conds {
		if conds[i].Type == condition.Type {
			// Existing condition found.
			if conds[i].ObservedGeneration == condition.ObservedGeneration &&
				conds[i].Status == condition.Status &&
				conds[i].Reason == condition.Reason &&
				conds[i].Message == condition.Message {
				return false
			}
			conds[i] = condition
			return true
		}
	}
	// No existing condition found, so add it.
	*conditions = append(*conditions, condition)
	return true
}

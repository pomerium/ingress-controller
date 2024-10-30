package gateway

import (
	context "context"
	"fmt"
	golog "log"

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
		// XXX: watch referenced certificate Secrets
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
	if err := c.List(ctx, &gcl); err != nil {
		return ctrl.Result{}, err
	}
	if err := c.List(ctx, &gl); err != nil {
		return ctrl.Result{}, err
	}
	if err := c.List(ctx, &hrl); err != nil {
		return ctrl.Result{}, err
	}
	// XXX: retries if any of those List calls fails?

	// Filter the hierarchy of Gateway objects to just the ones corresponding to this controller.
	// XXX: should we use some ListOptions in the List call to help with filtering?

	gcNames := set.New[string](0)
	for i := range gcl.Items {
		gc := &gcl.Items[i]
		if gc.Spec.ControllerName == gateway_v1.GatewayController(c.controllerName) {
			gcNames.Insert(gc.Name)
		}
	}

	var gateways []*gateway_v1.Gateway
	routesByParentRef := make(map[parentRefKey][]*gateway_v1.HTTPRoute)

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
			key := newParentRefKey(hr, pr)
			routesByParentRef[key] = append(routesByParentRef[key], hr)
		}
	}

	//var m model.GatewayConfig

	for _, g := range gateways {
		c.reconcileGateway(ctx, g, routesByParentRef[parentRefKeyForObject(g)])
	}

	return ctrl.Result{}, nil
}

// parentRefKey contains the essential fields of a ParentReference in a form suitable for use as a map key.
type parentRefKey struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func newParentRefKey(obj client.Object, ref *gateway_v1.ParentReference) parentRefKey {
	// Group defaults to the Gateway group name.
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.ParentReference.
	group := gateway_v1.GroupName
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	// Kind appears to have a default value in practice but I don't see this clearly spelled out
	// in the API reference.
	kind := "Gateway"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	// The namespace of a parentRef defaults to the object's own namespace.
	namespace := obj.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return parentRefKey{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      string(ref.Name),
	}
}

func parentRefKeyForObject(obj client.Object) parentRefKey {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return parentRefKey{
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
	// TODO: enforce permissions model

	// TODO: update Gateway status (Addresses, Listeners, Conditions)
	setGatewayCondition(gateway, metav1.Condition{
		Type:    string(gateway_v1.GatewayConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  "", // XXX
		Message: "", // XXX
	})
	// XXX: update status with API server

	// TODO: resolve route references, update status, translate to config

	// XXX: proper support for sectionName >>>
	// ParentRefs must be distinct. This means either that:
	// They select different objects. If this is the case, then parentRef entries are distinct. In terms of fields, this means that the multi-part key defined by group, kind, namespace, and name must be unique across all parentRef entries in the Route.
	// They do not select different objects, but for each optional field used, each ParentRef that selects the same object must set the same set of optional fields to different values. If one ParentRef sets a combination of optional fields, all must set the same combination.
	// Some examples:
	// If one ParentRef sets sectionName, all ParentRefs referencing the same object must also set sectionName.

	return nil
}

func setGatewayCondition(g *gateway_v1.Gateway, condition metav1.Condition) (modified bool) {
	condition.ObservedGeneration = g.Generation
	condition.LastTransitionTime = metav1.Now()

	conds := g.Status.Conditions
	for i := range conds {
		if conds[i].Type == condition.Type {
			// Existing condition found.
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

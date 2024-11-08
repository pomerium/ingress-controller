package gateway

import (
	context "context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	icgv1alpha1 "github.com/pomerium/ingress-controller/apis/gateway/v1alpha1"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium/gateway"
)

func (c *gatewayController) processExtensionFilters(
	ctx context.Context,
	config *model.GatewayConfig,
	o *objects,
) {
	for _, pf := range o.PolicyFilters {
		c.processPolicyFilter(ctx, pf)
	}
	config.ExtensionFilters = makeExtensionFilterMap(c.extensionFilters)
}

func (c *gatewayController) processPolicyFilter(ctx context.Context, pf *icgv1alpha1.PolicyFilter) {
	logger := log.FromContext(ctx)

	// Check to see if we already have a parsed representation of this filter.
	k := refKeyForObject(pf)
	f := c.extensionFilters[k]
	if f.object != nil && f.object.GetGeneration() == pf.Generation {
		return
	}

	filter, err := gateway.NewPolicyFilter(pf)

	// Set a "Valid" condition with information about whether the policy could be parsed.
	validCondition := metav1.Condition{
		Type: "Valid",
	}
	if err == nil {
		validCondition.Status = metav1.ConditionTrue
		validCondition.Reason = "Valid"
	} else {
		validCondition.Status = metav1.ConditionFalse
		validCondition.Reason = "Invalid"
		validCondition.Message = err.Error()
	}
	if upsertCondition(&pf.Status.Conditions, pf.Generation, validCondition) {
		if err := c.Status().Update(ctx, pf); err != nil {
			logger.Error(err, "couldn't update PolicyFilter status", "name", pf.Name)
		}
	}

	c.extensionFilters[k] = objectAndFilter{pf, filter}
}

type objectAndFilter struct {
	object client.Object
	filter model.ExtensionFilter
}

func makeExtensionFilterMap(
	extensionFilters map[refKey]objectAndFilter,
) map[model.ExtensionFilterKey]model.ExtensionFilter {
	m := make(map[model.ExtensionFilterKey]model.ExtensionFilter)
	for k, f := range extensionFilters {
		key := model.ExtensionFilterKey{Kind: k.Kind, Namespace: k.Namespace, Name: k.Name}
		m[key] = f.filter
	}
	return m
}

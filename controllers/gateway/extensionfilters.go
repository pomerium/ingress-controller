package gateway

import (
	context "context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	icgv1alpha1 "github.com/pomerium/ingress-controller/apis/gateway/v1alpha1"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium/gateway"
)

func (c *gatewayController) processExtensionFilters(
	ctx context.Context,
	config *model.GatewayConfig,
	o *objects,
) error {
	for _, pf := range o.PolicyFilters {
		if err := c.processPolicyFilter(ctx, pf); err != nil {
			return err
		}
	}
	config.ExtensionFilters = makeExtensionFilterMap(c.extensionFilters)
	return nil
}

func (c *gatewayController) processPolicyFilter(
	ctx context.Context,
	pf *icgv1alpha1.PolicyFilter,
) error {
	// Check to see if we already have a parsed representation of this filter.
	k := refKeyForObject(pf)
	f := c.extensionFilters[k]
	if f.object != nil && f.object.GetGeneration() == pf.Generation {
		return nil
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
			return fmt.Errorf("couldn't update status for PolicyFilter %q: %w", pf.Name, err)
		}
	}

	c.extensionFilters[k] = objectAndFilter{pf, filter}

	return nil
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

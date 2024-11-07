package gateway

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

func TestSettingsGenerationPredicateIgnoresStatusOnlyUpdates(t *testing.T) {
	p := predicate.GenerationChangedPredicate{}
	before := &icsv1.Pomerium{ObjectMeta: metav1.ObjectMeta{Name: "global", Generation: 7}}
	after := before.DeepCopy()
	after.ResourceVersion = "2"
	after.Status.Routes = map[string]icsv1.ResourceStatus{
		"default/app": {ObservedGeneration: 3, Reconciled: true},
	}

	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: before, ObjectNew: after}))
}

func TestSettingsGenerationPredicateAcceptsSpecUpdates(t *testing.T) {
	p := predicate.GenerationChangedPredicate{}
	before := &icsv1.Pomerium{ObjectMeta: metav1.ObjectMeta{Name: "global", Generation: 7}}
	after := before.DeepCopy()
	after.Generation = 8

	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: before, ObjectNew: after}))
}

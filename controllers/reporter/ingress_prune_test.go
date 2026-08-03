package reporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
)

func TestIngressSettingsReporterPrunesOnlyStaleStatuses(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, icsv1.AddToScheme(scheme))
	pomerium := &icsv1.Pomerium{
		ObjectMeta: metav1.ObjectMeta{Name: "global"},
		Status: icsv1.PomeriumStatus{Routes: map[string]icsv1.ResourceStatus{
			"apps/valid":       {Reconciled: true},
			"apps/invalid":     {Reconciled: false},
			"deleted/old":      {Reconciled: true},
			"deleted~team/old": {Reconciled: true},
		}},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&icsv1.Pomerium{}).
		WithObjects(pomerium).
		Build()
	reporter := &IngressSettingsReporter{SettingsReporter: SettingsReporter{
		NamespacedName: types.NamespacedName{Name: "global"},
		Client:         cl,
	}}

	err := reporter.PruneIngressStatuses(context.Background(), map[string]struct{}{
		"apps/valid":   {},
		"apps/invalid": {},
	})
	require.NoError(t, err)

	var updated icsv1.Pomerium
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: "global"}, &updated))
	require.Equal(t, map[string]icsv1.ResourceStatus{
		"apps/valid":   {Reconciled: true},
		"apps/invalid": {Reconciled: false},
	}, updated.Status.Routes)
}

func TestIngressSettingsReporterPruneIsNoopWithoutStaleStatuses(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, icsv1.AddToScheme(scheme))
	pomerium := &icsv1.Pomerium{
		ObjectMeta: metav1.ObjectMeta{Name: "global"},
		Status: icsv1.PomeriumStatus{Routes: map[string]icsv1.ResourceStatus{
			"apps/valid": {Reconciled: true},
		}},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&icsv1.Pomerium{}).
		WithObjects(pomerium).
		Build()
	reporter := &IngressSettingsReporter{SettingsReporter: SettingsReporter{
		NamespacedName: types.NamespacedName{Name: "global"},
		Client:         cl,
	}}

	require.NoError(t, reporter.PruneIngressStatuses(context.Background(), map[string]struct{}{
		"apps/valid": {},
	}))
}

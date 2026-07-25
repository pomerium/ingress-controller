package settings

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	controllers_mock "github.com/pomerium/ingress-controller/controllers/mock"
	"github.com/pomerium/ingress-controller/model"
)

func TestReconcileIgnoresDeletedPomerium(t *testing.T) {
	ctx := context.Background()
	name := types.NamespacedName{Name: "canary"}
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	mc.EXPECT().
		Get(ctx, name, gomock.AssignableToTypeOf(new(icsv1.Pomerium))).
		Return(apierrors.NewNotFound(schema.GroupResource{
			Group:    icsv1.GroupVersion.Group,
			Resource: "pomerium",
		}, name.Name))

	c := settingsController{
		key: model.Key{
			Kind:           "Pomerium",
			NamespacedName: name,
		},
		Client:   mc,
		Registry: model.NewRegistry(),
	}

	result, err := c.Reconcile(ctx, ctrl.Request{NamespacedName: name})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcileDoesNotReportReadErrorWithoutPomerium(t *testing.T) {
	ctx := context.Background()
	name := types.NamespacedName{Name: "canary"}
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	mc.EXPECT().
		Get(ctx, name, gomock.AssignableToTypeOf(new(icsv1.Pomerium))).
		Return(apierrors.NewForbidden(schema.GroupResource{
			Group:    icsv1.GroupVersion.Group,
			Resource: "pomerium",
		}, name.Name, assert.AnError))

	c := settingsController{
		key: model.Key{
			Kind:           "Pomerium",
			NamespacedName: name,
		},
		Client:   mc,
		Registry: model.NewRegistry(),
	}

	result, err := c.Reconcile(ctx, ctrl.Request{NamespacedName: name})

	require.Error(t, err)
	assert.ErrorContains(t, err, "get settings")
	assert.True(t, result.Requeue)
}

func TestReconcileRetriesMissingDependency(t *testing.T) {
	ctx := context.Background()
	name := types.NamespacedName{Name: "canary"}
	secretName := types.NamespacedName{Namespace: "pomerium", Name: "bootstrap-secrets"}
	mc := controllers_mock.NewMockClient(gomock.NewController(t))
	mc.EXPECT().
		Get(ctx, name, gomock.AssignableToTypeOf(new(icsv1.Pomerium))).
		Do(func(_ context.Context, _ types.NamespacedName, dst *icsv1.Pomerium, _ ...client.GetOption) {
			dst.Spec.Secrets = "pomerium/bootstrap-secrets"
		}).
		Return(nil)
	mc.EXPECT().
		Get(ctx, secretName, gomock.AssignableToTypeOf(new(corev1.Secret))).
		Return(apierrors.NewNotFound(schema.GroupResource{
			Resource: "secrets",
		}, secretName.Name))

	c := settingsController{
		key: model.Key{
			Kind:           "Pomerium",
			NamespacedName: name,
		},
		Client:   mc,
		Registry: model.NewRegistry(),
	}

	result, err := c.Reconcile(ctx, ctrl.Request{NamespacedName: name})

	require.Error(t, err)
	assert.ErrorContains(t, err, "bootstrap-secrets")
	assert.True(t, result.Requeue)
}

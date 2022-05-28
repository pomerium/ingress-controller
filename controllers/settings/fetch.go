//Package settings implements controller for Settings CRD
package settings

import (
	context "context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
)

// FetchConfig returns
func FetchConfig(ctx context.Context, client client.Client, name types.NamespacedName) (*model.Config, error) {
	var settings icsv1.Settings
	if err := client.Get(ctx, name, &settings); err != nil {
		return nil, fmt.Errorf("get %s: %w", name, err)
	}

	var cfg model.Config
	for _, apply := range []struct {
		name string
		src  *string
		dst  **corev1.Secret
	}{
		{"secret", &cfg.Spec.IdentityProvider.Secret, &cfg.IdpSecret},
		{"request params", cfg.Spec.IdentityProvider.RequestParamsSecret, &cfg.RequestParams},
		{"service account", cfg.Spec.IdentityProvider.ServiceAccountFromSecret, &cfg.IdpServiceAccount},
	} {
		if apply.src == nil {
			continue
		}
		name, err := util.ParseNamespacedName(*apply.src, util.WithDefaultNamespace(settings.Namespace))
		if err != nil {
			return nil, fmt.Errorf("parse %s %s: %w", apply.name, *apply.src, err)
		}
		var secret corev1.Secret
		if err := client.Get(ctx, *name, &secret); err != nil {
			return nil, fmt.Errorf("get %s: %w", name, err)
		}
		*apply.dst = &secret
	}

	return &cfg, nil
}

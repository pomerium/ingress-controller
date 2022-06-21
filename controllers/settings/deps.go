package settings

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/pomerium/ingress-controller/model"
)

func (c *settingsController) updateDependencies(cfg *model.Config) {
	updateDependencies(cfg, c.Registry, c.Scheme)
}

func updateDependencies(cfg *model.Config, r model.Registry, scheme *runtime.Scheme) {
	key := model.ObjectKey(&cfg.Settings, scheme)
	r.DeleteCascade(key)

	for _, s := range cfg.Certs {
		r.Add(key, model.ObjectKey(s, scheme))
	}

	for _, s := range []*corev1.Secret{
		cfg.IdpSecret,
		cfg.IdpServiceAccount,
		cfg.RequestParams,
		cfg.StorageSecrets.Secret,
		cfg.StorageSecrets.TLS,
		cfg.StorageSecrets.CA,
	} {
		if s == nil {
			continue
		}
		r.Add(key, model.ObjectKey(s, scheme))
	}
}

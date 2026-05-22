package certificate

import "k8s.io/apimachinery/pkg/types"

// DefaultControllerName is the default controller name.
var DefaultControllerName = "pomerium-certificate"

type controllerConfig struct {
	controllerName     string
	globalSettingsName types.NamespacedName
	// if not set, discover the namespace from the issuer or the pod where the
	// controller is running
	namespace *string
}

// An Option customizes the config.
type Option func(cfg *controllerConfig)

// WithControllerName sets the controller name in the config.
func WithControllerName(controllerName string) Option {
	return func(cfg *controllerConfig) {
		cfg.controllerName = controllerName
	}
}

// WithGlobalSettingsName sets the global settings name in the config.
func WithGlobalSettingsName(globalSettingsName types.NamespacedName) Option {
	return func(cfg *controllerConfig) {
		cfg.globalSettingsName = globalSettingsName
	}
}

// WithNamespace sets the namespace option in the config.
func WithNamespace(namespace string) Option {
	return func(cfg *controllerConfig) {
		cfg.namespace = new(namespace)
	}
}

func getControllerConfig(options ...Option) *controllerConfig {
	cfg := new(controllerConfig)
	WithControllerName(DefaultControllerName)(cfg)
	for _, o := range options {
		o(cfg)
	}
	return cfg
}

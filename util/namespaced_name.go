// Package util contains misc utils
package util

import (
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	// ErrInvalidNamespacedNameFormat namespaced name format error
	ErrInvalidNamespacedNameFormat = errors.New("invalid format, expect name or namespace/name")
	// ErrNamespaceExpected indicates that namespace must be provided
	ErrNamespaceExpected = errors.New("missing namespace for resource")
	// ErrEmptyName indicates the resource must be non-empty
	ErrEmptyName = errors.New("resource name cannot be blank")
)

// NamespacedNameOption customizes namespaced name parsing
type NamespacedNameOption func(name *types.NamespacedName) error

// WithNamespaceExpected will set namespace to provided default, if missing
func WithNamespaceExpected() NamespacedNameOption {
	return func(name *types.NamespacedName) error {
		if name.Namespace == "" {
			return ErrNamespaceExpected
		}
		return nil
	}
}

// WithDefaultNamespace will set namespace to provided default, if missing
func WithDefaultNamespace(namespace string) NamespacedNameOption {
	return func(name *types.NamespacedName) error {
		if namespace == "" {
			return ErrNamespaceExpected
		}

		if name.Namespace == "" {
			name.Namespace = namespace
		}
		return nil
	}
}

// WithMustNamespace enforces the namespace to match provided one
func WithMustNamespace(namespace string) NamespacedNameOption {
	return func(name *types.NamespacedName) error {
		if namespace == "" {
			return ErrNamespaceExpected
		}

		if name.Namespace == "" {
			name.Namespace = namespace
		} else if name.Namespace != namespace {
			return fmt.Errorf("expected namespace %s, got %s", namespace, name.Namespace)
		}
		return nil
	}
}

// WithClusterScope ensures the name is not namespaced
func WithClusterScope() NamespacedNameOption {
	return func(name *types.NamespacedName) error {
		if name.Namespace != "" {
			return fmt.Errorf("expected cluster-scoped name")
		}
		return nil
	}
}

// ParseNamespacedName parses "namespace/name" or "name" format
func ParseNamespacedName(name string, options ...NamespacedNameOption) (*types.NamespacedName, error) {
	if len(options) > 1 {
		return nil, errors.New("at most one option may be supplied")
	}

	if len(options) == 0 {
		options = []NamespacedNameOption{WithNamespaceExpected()}
	}

	if name == "" {
		return nil, ErrEmptyName
	}

	parts := strings.Split(name, "/")
	var dst types.NamespacedName
	switch len(parts) {
	case 1:
		dst.Name = parts[0]
	case 2:
		dst.Namespace = parts[0]
		dst.Name = parts[1]
	default:
		return nil, ErrInvalidNamespacedNameFormat
	}

	for _, opt := range options {
		if err := opt(&dst); err != nil {
			return nil, err
		}
	}

	if dst.Name == "" {
		return nil, ErrInvalidNamespacedNameFormat
	}

	return &dst, nil
}

// GetNamespacedName a convenience method to return types.NamespacedName for an object
func GetNamespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}

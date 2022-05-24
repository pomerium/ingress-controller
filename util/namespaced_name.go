// Package util contains misc utils
package util

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
)

var (
	// ErrInvalidNamespacedNameFormat namespaced name format error
	ErrInvalidNamespacedNameFormat = errors.New("invalid format, expect name or namespace/name")
)

// NamespacedNameOption customizes namespaced name parsing
type NamespacedNameOption func(name *types.NamespacedName) error

// WithDefaultNamespace will set namespace to provided default, if missing
func WithDefaultNamespace(namespace string) NamespacedNameOption {
	return func(name *types.NamespacedName) error {
		if name.Namespace == "" {
			name.Namespace = namespace
		}
		return nil
	}
}

// WithMustNamespace enforces the namespace to match provided one
func WithMustNamespace(namespace string) NamespacedNameOption {
	return func(name *types.NamespacedName) error {
		if name.Namespace == "" {
			name.Namespace = namespace
		} else if name.Namespace != namespace {
			return fmt.Errorf("expected namespace %s, got %s", namespace, name.Namespace)
		}
		return nil
	}
}

// ParseNamespacedName parses "namespace/name" or "name" format
func ParseNamespacedName(name string, options ...NamespacedNameOption) (*types.NamespacedName, error) {
	if len(options) > 1 {
		return nil, errors.New("at most one option may be supplied")
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

	if dst.Name == "" || dst.Namespace == "" {
		return nil, ErrInvalidNamespacedNameFormat
	}

	return &dst, nil
}

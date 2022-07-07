package util_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/util"
)

func TestParseNamespacedName(t *testing.T) {
	for _, tc := range []struct {
		in       string
		opts     []util.NamespacedNameOption
		want     *types.NamespacedName
		errCheck func(require.TestingT, error, ...interface{})
	}{{
		"no_namespace",
		nil,
		nil,
		require.Error,
	}, {
		"",
		nil,
		nil,
		require.Error,
	}, {
		"empty_default_namespace",
		[]util.NamespacedNameOption{util.WithDefaultNamespace("")},
		nil,
		require.Error,
	}, {
		"empty_required_namespace",
		[]util.NamespacedNameOption{util.WithMustNamespace("")},
		nil,
		require.Error,
	}, {
		"with_must_namespace",
		[]util.NamespacedNameOption{util.WithMustNamespace("default")},
		&types.NamespacedName{Namespace: "default", Name: "with_must_namespace"},
		require.NoError,
	}, {
		"empty_must_namespace",
		[]util.NamespacedNameOption{util.WithMustNamespace("")},
		nil,
		require.Error,
	}, {
		"with_default_namespace",
		[]util.NamespacedNameOption{util.WithDefaultNamespace("default")},
		&types.NamespacedName{Namespace: "default", Name: "with_default_namespace"},
		require.NoError,
	}, {
		"pomerium/name",
		[]util.NamespacedNameOption{util.WithDefaultNamespace("default")},
		&types.NamespacedName{Namespace: "pomerium", Name: "name"},
		require.NoError,
	}, {
		"cluster-scoped",
		[]util.NamespacedNameOption{util.WithClusterScope()},
		&types.NamespacedName{Name: "cluster-scoped"},
		require.NoError,
	}, {
		"pomerium/name",
		[]util.NamespacedNameOption{util.WithClusterScope()},
		nil,
		require.Error,
	}, {
		"wrong/format/here",
		nil,
		nil,
		require.Error,
	}, {
		"enforced_namespace/name",
		[]util.NamespacedNameOption{util.WithMustNamespace("pomerium")},
		nil,
		require.Error,
	}} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := util.ParseNamespacedName(tc.in, tc.opts...)
			tc.errCheck(t, err, "errcheck")
			assert.Equal(t, tc.want, got)
		})
	}
}

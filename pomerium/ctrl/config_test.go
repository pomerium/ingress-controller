package ctrl_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pomerium/pomerium/config"

	"github.com/pomerium/ingress-controller/pomerium/ctrl"
)

func TestConfigChangeDetect(t *testing.T) {
	cfg, err := ctrl.NewConfigSource()
	require.NoError(t, err)

	ctx := context.Background()
	def := *config.NewDefaultOptions()
	for _, tc := range []struct {
		msg    string
		expect bool
		config.Options
	}{
		{"initial", true, def},
		{"same initial", false, def},
		{"same again", false, def},
		{"changed", true, config.Options{}},
	} {
		assert.Equal(t, tc.expect, cfg.SetOptions(ctx, tc.Options), tc.msg)
	}
}

package ctrl_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pomerium/pomerium/config"

	"github.com/pomerium/ingress-controller/pomerium/ctrl"
)

func TestConfigChangeDetect(t *testing.T) {
	cfg := new(ctrl.InMemoryConfigSource)

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
		assert.Equal(t, tc.expect, cfg.SetConfig(ctx, &config.Config{Options: &tc.Options}), tc.msg)
	}
}

package cmd

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIngressControllerOptsAdaptiveBatchFlags(t *testing.T) {
	var opts ingressControllerOpts
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opts.setupFlags(flags)
	require.NoError(t, flags.Parse([]string{
		"--reconcile-batch-window=1s",
		"--reconcile-batch-max-wait=5s",
	}))
	opts.GlobalSettings = "default/pomerium"
	require.NoError(t, opts.Validate())
	assert.Equal(t, time.Second, opts.ReconcileBatchWindow)
	assert.Equal(t, 5*time.Second, opts.ReconcileBatchMaxWait)
}

func TestIngressControllerOptsRejectsBatchMaxWaitBelowWindow(t *testing.T) {
	var opts ingressControllerOpts
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opts.setupFlags(flags)
	require.NoError(t, flags.Parse([]string{
		"--reconcile-batch-window=5s",
		"--reconcile-batch-max-wait=1s",
	}))
	opts.GlobalSettings = "default/pomerium"
	require.ErrorContains(t, opts.Validate(), "--reconcile-batch-max-wait")
}

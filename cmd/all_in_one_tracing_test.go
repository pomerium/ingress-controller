package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pomerium/pomerium/pkg/telemetry/trace"
)

// TestAllInOneTracingStartup is a regression guard for issue #1465.
//
// all-in-one mode panicked at startup ("conflicting Schema URL") because the
// OpenTelemetry SDK's resource.Default() adopted a newer semconv schema URL than
// the one pomerium's tracing package merges it with. This reproduces the exact
// startup sequence from (*allCmdParam).run: install the trace system context, then
// build the tracer provider as pomerium core does (pkg/cmd/pomerium.Run ->
// trace.NewTracerProvider). By exercising pomerium's real trace package it fails
// again if any future otel-sdk / pomerium dependency bump reintroduces the skew.
func TestAllInOneTracingStartup(t *testing.T) {
	ctx := trace.NewContext(context.Background(), trace.NewSyncClient(nil))
	t.Cleanup(func() { _ = trace.ShutdownContext(ctx) })

	assert.NotPanics(t, func() {
		_ = trace.NewTracerProvider(ctx, "Pomerium")
	})
}

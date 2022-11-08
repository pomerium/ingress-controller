package util_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pomerium/ingress-controller/util"
)

type testType string

func TestBin(t *testing.T) {
	ctx := util.WithBin[testType](context.Background())
	util.Add(ctx, testType("test"))
	require.Equal(t, []testType{"test"}, util.Get[testType](ctx))
}

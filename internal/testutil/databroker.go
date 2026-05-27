package testutil

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/databroker"
	databrokerpb "github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"
)

// NewInMemoryDataBroker creates a new in-memory databroker.
func NewInMemoryDataBroker(tb testing.TB) databrokerpb.DataBrokerServiceClient {
	key := bytes.Repeat([]byte{0x01}, 32)
	cfg := config.New(config.NewDefaultOptions())
	cfg.Options.SharedKey = base64.StdEncoding.EncodeToString(key)
	srv := databroker.NewServer(noop.NewTracerProvider(), cfg)
	srv.OnConfigChange(tb.Context(), cfg)
	tb.Cleanup(srv.Stop)

	cc := NewGRPCServer(tb,
		func(s *grpc.Server) {
			databrokerpb.RegisterDataBrokerServiceServer(s, srv)
		},
		grpc.WithStreamInterceptor(grpcutil.WithStreamSignedJWT(func() []byte { return key })),
		grpc.WithUnaryInterceptor(grpcutil.WithUnarySignedJWT(func() []byte { return key })),
	)

	return databrokerpb.NewDataBrokerServiceClient(cc)
}

// NewGRPCServer starts a gRPC server and returns a client connection to it.
func NewGRPCServer(
	t testing.TB,
	register func(s *grpc.Server),
	dialOpts ...grpc.DialOption,
) *grpc.ClientConn {
	t.Helper()

	li := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	register(s)
	go func() {
		err := s.Serve(li)
		if errors.Is(err, grpc.ErrServerStopped) {
			err = nil
		}
		require.NoError(t, err)
	}()
	t.Cleanup(func() {
		s.Stop()
	})

	opts := []grpc.DialOption{
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return li.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	opts = append(opts, dialOpts...)

	cc, err := grpc.NewClient("passthrough://bufnet", opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		cc.Close()
	})

	return cc
}

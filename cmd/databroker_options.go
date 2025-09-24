package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"

	"github.com/spf13/pflag"
	"github.com/volatiletech/null/v9"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/pkg/grpcutil"
)

type dataBrokerOptions struct {
	ClusterNodeID   string
	ClusterNodes    dataBrokerClusterNodes
	RaftBindAddress string   `validate:"omitempty,hostname_port"`
	ServiceURLs     []string `validate:"dive,url"`
}

// dataBrokerClusterNodes is a custom type to support reading cluster nodes from yaml
type dataBrokerClusterNodes []config.DataBrokerClusterNode

func (n dataBrokerClusterNodes) MarshalText() ([]byte, error) {
	return yaml.Marshal([]config.DataBrokerClusterNode(n))
}

func (n *dataBrokerClusterNodes) UnmarshalText(text []byte) error {
	return yaml.Unmarshal(text, (*[]config.DataBrokerClusterNode)(n))
}

func (o *dataBrokerOptions) setupFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.ClusterNodeID, "databroker-cluster-node-id", "", "the databroker cluster node id")
	flags.TextVar(&o.ClusterNodes, "databroker-cluster-nodes", dataBrokerClusterNodes(nil), "the databroker cluster nodes")
	flags.StringVar(&o.RaftBindAddress, "databroker-raft-bind-address", "", "the databroker raft bind address")
	flags.StringSliceVar(&o.ServiceURLs, "databroker-service-urls", nil, "the databroker service urls, defaults to localhost")
}

func (o *dataBrokerOptions) apply(dst *config.Config) {
	if o.ClusterNodeID != "" {
		dst.Options.DataBroker.ClusterNodeID = null.StringFrom(o.ClusterNodeID)
	}
	dst.Options.DataBroker.ClusterNodes = config.DataBrokerClusterNodes(o.ClusterNodes)
	if o.RaftBindAddress != "" {
		dst.Options.DataBroker.RaftBindAddress = null.StringFrom(o.RaftBindAddress)
	}
	dst.Options.DataBroker.ServiceURLs = o.ServiceURLs
}

func getDataBrokerConnection(ctx context.Context, cfg *config.Config) (*grpc.ClientConn, error) {
	sharedSecret, err := base64.StdEncoding.DecodeString(cfg.Options.SharedKey)
	if err != nil {
		return nil, fmt.Errorf("decode shared_secret: %w", err)
	}

	// use the local gRPC port
	dataBrokerURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", cfg.GRPCPort)}
	// if a databroker service url is set, use the outbound port instead
	if len(cfg.Options.DataBroker.ServiceURLs) > 0 || cfg.Options.DataBroker.ServiceURL != "" {
		dataBrokerURL = &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", cfg.OutboundPort)}
	}

	return grpcutil.NewGRPCClientConn(ctx, &grpcutil.Options{
		Address:        dataBrokerURL,
		ServiceName:    "databroker",
		SignedJWTKey:   sharedSecret,
		RequestTimeout: defaultGRPCTimeout,
	})
}

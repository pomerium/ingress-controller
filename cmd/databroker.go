package cmd

import (
	"github.com/spf13/pflag"
	"github.com/volatiletech/null/v9"
	"gopkg.in/yaml.v3"

	"github.com/pomerium/pomerium/config"
)

type dataBrokerOptions struct {
	clusterLeaderID string
	clusterNodeID   string
	clusterNodes    dataBrokerClusterNodes
	raftBindAddress string `validate:"hostname_port"`
	serviceURLs     []string
}

type dataBrokerClusterNodes []config.DataBrokerClusterNode

func (n dataBrokerClusterNodes) MarshalText() ([]byte, error) {
	return yaml.Marshal([]config.DataBrokerClusterNode(n))
}

func (n *dataBrokerClusterNodes) UnmarshalText(text []byte) error {
	return yaml.Unmarshal(text, (*[]config.DataBrokerClusterNode)(n))
}

func (o *dataBrokerOptions) setupFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.clusterLeaderID, "databroker-cluster-leader-id", "", "the databroker cluster leader id")
	flags.StringVar(&o.clusterNodeID, "databroker-cluster-node-id", "", "the databroker cluster node id")
	flags.TextVar(&o.clusterNodes, "databroker-cluster-nodes", dataBrokerClusterNodes(nil), "the databroker cluster nodes")
	flags.StringVar(&o.raftBindAddress, "databroker-raft-bind-address", "", "the databroker raft bind address")
	flags.StringSliceVar(&o.serviceURLs, "databroker-service-urls", nil, "the databroker service urls, defaults to localhost")
}

func (o *dataBrokerOptions) apply(dst *config.Config) {
	if o.clusterLeaderID != "" {
		dst.Options.DataBroker.ClusterLeaderID = null.StringFrom(o.clusterLeaderID)
	}
	if o.clusterNodeID != "" {
		dst.Options.DataBroker.ClusterNodeID = null.StringFrom(o.clusterNodeID)
	}
	dst.Options.DataBroker.ClusterNodes = config.DataBrokerClusterNodes(o.clusterNodes)
	if o.raftBindAddress != "" {
		dst.Options.DataBroker.RaftBindAddress = null.StringFrom(o.raftBindAddress)
	}
	dst.Options.DataBroker.ServiceURLs = o.serviceURLs
}

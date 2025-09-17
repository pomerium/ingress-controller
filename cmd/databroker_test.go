package cmd

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/volatiletech/null/v9"
)

func TestDataBrokerOptions(t *testing.T) {
	t.Parallel()

	options := &dataBrokerOptions{}

	flags := pflag.NewFlagSet("test", pflag.PanicOnError)
	options.setupFlags(flags)
	err := flags.Parse([]string{
		"--databroker-cluster-leader-id", "node-1",
		"--databroker-cluster-node-id", "node-2",
		"--databroker-cluster-nodes", `[
			{ "id": "node-0", "grpc_address": "127.0.0.1:15000", "raft_address": "127.0.0.1:15100" },
			{ "id": "node-1", "grpc_address": "127.0.0.1:15001", "raft_address": "127.0.0.1:15101" },
			{ "id": "node-2", "grpc_address": "127.0.0.1:15002", "raft_address": "127.0.0.1:15102" }
		]`,
	})
	assert.NoError(t, err)

	assert.Equal(t, &dataBrokerOptions{
		clusterLeaderID: "node-1",
		clusterNodeID:   "node-2",
		clusterNodes: dataBrokerClusterNodes{
			{ID: "node-0", GRPCAddress: "127.0.0.1:15000", RaftAddress: null.StringFrom("127.0.0.1:15100")},
			{ID: "node-1", GRPCAddress: "127.0.0.1:15001", RaftAddress: null.StringFrom("127.0.0.1:15101")},
			{ID: "node-2", GRPCAddress: "127.0.0.1:15002", RaftAddress: null.StringFrom("127.0.0.1:15102")},
		},
	}, options)
}

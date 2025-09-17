package cmd

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/volatiletech/null/v9"
)

func TestDataBrokerOptions(t *testing.T) {
	t.Parallel()

	parseOptions := func(arguments []string) *dataBrokerOptions {
		flags := pflag.NewFlagSet("test", pflag.PanicOnError)
		options := &dataBrokerOptions{}
		options.setupFlags(flags)
		err := flags.Parse(arguments)
		assert.NoError(t, err)
		return options
	}

	options := parseOptions([]string{
		"--databroker-cluster-leader-id", "node-1",
		"--databroker-cluster-node-id", "node-2",
		"--databroker-cluster-nodes", `[
			{ "id": "node-0", "grpc_address": "127.0.0.1:15000", "raft_address": "127.0.0.1:15100" },
			{ "id": "node-1", "grpc_address": "127.0.0.1:15001", "raft_address": "127.0.0.1:15101" },
			{ "id": "node-2", "grpc_address": "127.0.0.1:15002", "raft_address": "127.0.0.1:15102" }
		]`,
		"--databroker-raft-bind-address=:15001",
		"--databroker-service-urls=https://databroker-1.example.com",
		"--databroker-service-urls=https://databroker-2.example.com",
		"--databroker-service-urls=https://databroker-3.example.com",
	})
	assert.Equal(t, &dataBrokerOptions{
		ClusterLeaderID: "node-1",
		ClusterNodeID:   "node-2",
		ClusterNodes: dataBrokerClusterNodes{
			{ID: "node-0", GRPCAddress: "127.0.0.1:15000", RaftAddress: null.StringFrom("127.0.0.1:15100")},
			{ID: "node-1", GRPCAddress: "127.0.0.1:15001", RaftAddress: null.StringFrom("127.0.0.1:15101")},
			{ID: "node-2", GRPCAddress: "127.0.0.1:15002", RaftAddress: null.StringFrom("127.0.0.1:15102")},
		},
		RaftBindAddress: ":15001",
		ServiceURLs: []string{
			"https://databroker-1.example.com",
			"https://databroker-2.example.com",
			"https://databroker-3.example.com",
		},
	}, options)
	assert.NoError(t, validator.New().Struct(options))

	assert.NoError(t, validator.New().Struct(parseOptions([]string{})))
	assert.Error(t, validator.New().Struct(parseOptions([]string{
		"--databroker-raft-bind-address", "<INVALID>",
	})))
	assert.Error(t, validator.New().Struct(parseOptions([]string{
		"--databroker-service-urls", "<INVALID>",
	})))
}

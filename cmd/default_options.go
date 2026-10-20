package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/pflag"
	"github.com/volatiletech/null/v9"

	"github.com/pomerium/pomerium/config"
)

type defaultOptions struct {
	enabled bool
}

func (o *defaultOptions) setupFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&o.enabled, "default-config", false, "configure pomerium using the default set of config options")
}

func (o *defaultOptions) apply(dst *config.Config) {
	if !o.enabled {
		return
	}

	replicas := o.getReplicas()
	podName := o.getPodName()

	// configure metrics
	dst.Options.MetricsAddr = fmt.Sprintf("%s:9090", o.getPodIP())

	// configure health checks
	dst.Options.HealthCheckAddr = fmt.Sprintf("%s:28080", o.getPodIP())

	// configure the databroker
	dst.Options.DataBroker.ClusterNodeID = null.StringFrom(podName)
	dst.Options.DataBroker.ClusterNodes = nil
	for i := range replicas {
		dst.Options.DataBroker.ClusterNodes = append(dst.Options.DataBroker.ClusterNodes, config.DataBrokerClusterNode{
			ID:          fmt.Sprintf("pomerium-%d", i),
			GRPCAddress: fmt.Sprintf("https://pomerium-%d.pomerium-headless:5443", i),
			RaftAddress: null.StringFrom(fmt.Sprintf("pomerium-%d.pomerium-headless:5999", i)),
		})
	}
	if replicas == 1 {
		// if there is only a single replica, set the leader id and disable raft
		dst.Options.DataBroker.ClusterLeaderID = dst.Options.DataBroker.ClusterNodeID
		dst.Options.DataBroker.RaftBindAddress = null.NewString("", false)
	} else {
		// if there are multiple nodes, use raft leader election
		dst.Options.DataBroker.RaftBindAddress = null.StringFrom(":5999")
		dst.Options.DeriveInternalDomainCert = new("*.pomerium-headless")
		dst.Options.GRPCInsecure = new(false)
	}
}

func (o *defaultOptions) getReplicas() int {
	if v, err := strconv.Atoi(os.Getenv("REPLICAS")); err == nil {
		return v
	}
	return 1
}

func (o *defaultOptions) getPodIP() string {
	return os.Getenv("POD_IP")
}

func (o *defaultOptions) getPodName() string {
	if v := os.Getenv("POD_NAME"); v != "" {
		return v
	}
	return "pomerium-0"
}

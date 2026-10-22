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

func (o *defaultOptions) setupFlags(_ *pflag.FlagSet) {
	o.enabled = os.Getenv("REPLICAS") != ""
}

func (o *defaultOptions) apply(dst *config.Config) {
	if !o.enabled {
		return
	}

	replicas := o.getReplicas()
	podName := o.getPodName()

	// configure the databroker
	dst.Options.DataBroker.ClusterNodeID = null.StringFrom(podName)
	dst.Options.DataBroker.ClusterNodes = nil
	dst.Options.DataBroker.ServiceURL = ""
	dst.Options.DataBroker.ServiceURLs = nil
	for i := range replicas {
		dst.Options.DataBroker.ClusterNodes = append(dst.Options.DataBroker.ClusterNodes, config.DataBrokerClusterNode{
			ID:          fmt.Sprintf("pomerium-%d", i),
			GRPCAddress: fmt.Sprintf("https://pomerium-%d.pomerium-headless:5443", i),
			RaftAddress: null.StringFrom(fmt.Sprintf("pomerium-%d.pomerium-headless:5999", i)),
		})
		dst.Options.DataBroker.ServiceURLs = append(dst.Options.DataBroker.ServiceURLs,
			fmt.Sprintf("https://pomerium-%d.pomerium-headless:5443", i))
	}
	dst.Options.DataBroker.RaftBindAddress = null.StringFrom(":5999")
	dst.Options.DeriveInternalDomainCert = new("*.pomerium-headless")
	dst.Options.GRPCInsecure = new(false)
	if replicas == 1 {
		// if there is only a single replica, set the leader id explicitly
		dst.Options.DataBroker.ClusterLeaderID = dst.Options.DataBroker.ClusterNodeID
	}
}

func (o *defaultOptions) getReplicas() int {
	if v, err := strconv.Atoi(os.Getenv("REPLICAS")); err == nil {
		return v
	}
	return 1
}

func (o *defaultOptions) getPodName() string {
	if v := os.Getenv("POD_NAME"); v != "" {
		return v
	}
	return "pomerium-0"
}

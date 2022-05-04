package pomerium

import (
	"context"
	"encoding/base64"
	"errors"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/config/envoyconfig"
	"github.com/pomerium/pomerium/config/envoyconfig/filemgr"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/pomerium/envoy"
)

// validate validates pomerium config.
func validate(ctx context.Context, cfg *pb.Config, id string) error {
	options := config.NewDefaultOptions()
	options.ApplySettings(ctx, cfg.GetSettings())
	options.InsecureServer = true
	options.ServiceAccount = base64.StdEncoding.EncodeToString([]byte(`{}`))

	for _, r := range cfg.GetRoutes() {
		p, err := config.NewPolicyFromProto(r)
		if err != nil {
			return err
		}
		err = p.Validate()
		if err != nil {
			return err
		}
		options.Policies = append(options.Policies, *p)
	}

	err := options.Validate()
	if err != nil {
		return err
	}

	pCfg := &config.Config{Options: options, OutboundPort: "8002"}

	builder := envoyconfig.New("127.0.0.1:8000", "127.0.0.1:8001", "127.0.0.1:8003", filemgr.NewManager(), nil)

	bootstrapCfg := new(envoy_config_bootstrap_v3.Bootstrap)
	bootstrapCfg.Admin, err = builder.BuildBootstrapAdmin(pCfg)
	if err != nil {
		return err
	}

	bootstrapCfg.StaticResources, err = builder.BuildBootstrapStaticResources()
	if err != nil {
		return err
	}

	clusters, err := builder.BuildClusters(ctx, pCfg)
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		if cluster.Name == "pomerium-control-plane-grpc" {
			continue
		}
		bootstrapCfg.StaticResources.Clusters = append(bootstrapCfg.StaticResources.Clusters, cluster)
	}

	listeners, err := builder.BuildListeners(ctx, pCfg)
	if err != nil {
		return err
	}
	bootstrapCfg.StaticResources.Listeners = append(bootstrapCfg.StaticResources.Listeners, listeners...)

	res, err := envoy.Validate(ctx, bootstrapCfg, id)
	if err != nil {
		return err
	}
	if !res.Valid {
		return errors.New(res.Message)
	}

	return nil
}

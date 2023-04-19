package pomerium

import (
	"context"
	"errors"

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
	bootstrap, err := builder.BuildBootstrap(ctx, pCfg, true)
	if err != nil {
		return err
	}

	res, err := envoy.Validate(ctx, bootstrap, id)
	if err != nil {
		return err
	}
	if !res.Valid {
		return errors.New(res.Message)
	}

	return nil
}

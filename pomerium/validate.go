package pomerium

import (
	"context"
	"errors"

	"golang.org/x/net/nettest"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/config/envoyconfig"
	"github.com/pomerium/pomerium/config/envoyconfig/filemgr"
	"github.com/pomerium/pomerium/pkg/cryptutil"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/pomerium/envoy"
)

// validateOptions builds and validates pomerium options from the config without
// invoking the (expensive) embedded envoy subprocess. It catches route/policy and
// option-level errors cheaply.
func validateOptions(ctx context.Context, cfg *pb.Config) (*config.Options, error) {
	options := config.NewDefaultOptions()
	options.ApplySettings(ctx, cryptutil.NewCertificatesIndex(), cfg.GetSettings())
	options.InsecureServer = true

	for _, r := range cfg.GetRoutes() {
		p, err := config.NewPolicyFromProto(r)
		if err != nil {
			return nil, err
		}
		err = p.Validate()
		if err != nil {
			return nil, err
		}
		options.Policies = append(options.Policies, *p)
	}

	if err := options.Validate(); err != nil {
		return nil, err
	}

	return options, nil
}

// validateCheap validates a pomerium config without invoking the embedded envoy
// subprocess. It is used to catch most invalid ingresses quickly, deferring the
// single expensive full-config envoy validation to saveConfig.
func validateCheap(ctx context.Context, cfg *pb.Config) error {
	_, err := validateOptions(ctx, cfg)
	return err
}

// validate validates pomerium config, including a full bootstrap validation via
// the embedded envoy binary.
func validate(ctx context.Context, cfg *pb.Config, id string) error {
	options, err := validateOptions(ctx, cfg)
	if err != nil {
		return err
	}

	pCfg := config.New(options)
	pCfg.OutboundPort = "8002"

	builder := envoyconfig.New("127.0.0.1:8000",
		"127.0.0.1:8001",
		"127.0.0.1:8003",
		"127.0.0.1:8004",
		"127.0.0.1:8005",
		filemgr.NewManager(),
		nil,
		nettest.SupportsIPv6())
	bootstrap, err := builder.BuildBootstrap(ctx, pCfg, true, nil)
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

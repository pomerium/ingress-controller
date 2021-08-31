// Package pomerium implements logic to convert K8s objects into Pomerium configuration
package pomerium

import (
	"bytes"
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/protoutil"

	"github.com/pomerium/ingress-controller/model"
)

const (
	configID = "ingress-controller"
)

// ConfigReconciler updates pomerium configuration
// only one ConfigReconciler should be active
// and its methods are not thread-safe
type ConfigReconciler struct {
	databroker.DataBrokerServiceClient
}

// Upsert should update or create the pomerium routes corresponding to this ingress
func (r *ConfigReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) error {
	cfg, prevBytes, err := r.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	if err := upsertRoutes(ctx, cfg, ic); err != nil {
		return fmt.Errorf("deleting pomerium config records: %w", err)
	}
	if err := upsertCerts(cfg, ic); err != nil {
		return fmt.Errorf("updating certs: %w", err)
	}
	if err := r.saveConfig(ctx, cfg, prevBytes); err != nil {
		return fmt.Errorf("updating pomerium config: %w", err)
	}
	return nil
}

// Delete should delete pomerium routes corresponding to this ingress name
func (r *ConfigReconciler) Delete(ctx context.Context, namespacedName types.NamespacedName) error {
	cfg, prevBytes, err := r.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("get pomerium config: %w", err)
	}
	if err := deleteRoutes(ctx, cfg, namespacedName); err != nil {
		return fmt.Errorf("deleting pomerium config records %s: %w", namespacedName.String(), err)
	}
	if err := removeUnusedCerts(cfg); err != nil {
		return fmt.Errorf("removing unused certs: %w", err)
	}
	if err := r.saveConfig(ctx, cfg, prevBytes); err != nil {
		return fmt.Errorf("updating pomerium config: %w", err)
	}
	return nil
}

func (r *ConfigReconciler) getConfig(ctx context.Context) (*pomerium.Config, []byte, error) {
	cfg := new(pomerium.Config)
	any := protoutil.NewAny(cfg)
	var hdr metadata.MD
	resp, err := r.Get(ctx, &databroker.GetRequest{
		Type: any.GetTypeUrl(),
		Id:   configID,
	}, grpc.Header(&hdr))
	if status.Code(err) == codes.NotFound {
		return &pomerium.Config{}, nil, nil
	} else if err != nil {
		return nil, nil, fmt.Errorf("get pomerium config: %w", err)
	}

	if err := resp.GetRecord().GetData().UnmarshalTo(cfg); err != nil {
		return nil, nil, fmt.Errorf("unmarshal current config: %w", err)
	}

	fmt.Println("<=", protojson.Format(cfg))

	return cfg, resp.GetRecord().GetData().GetValue(), nil
}

func (r *ConfigReconciler) saveConfig(ctx context.Context, cfg *pomerium.Config, prevBytes []byte) error {
	logger := log.FromContext(ctx)
	any := protoutil.NewAny(cfg)

	if bytes.Equal(prevBytes, any.GetValue()) {
		logger.Info("no changes in pomerium config")
		return nil
	}

	if _, err := r.Put(ctx, &databroker.PutRequest{
		Record: &databroker.Record{
			Type: any.GetTypeUrl(),
			Id:   configID,
			Data: any,
		},
	}); err != nil {
		return err
	}

	logger.Info("new pomerium config applied")
	// TODO: rm
	fmt.Println("=>", protojson.Format(cfg))

	return nil
}

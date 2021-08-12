package reconciler

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/controllers"
	pomerium "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/protoutil"
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
func (r *ConfigReconciler) Upsert(ctx context.Context, ing *networkingv1.Ingress, tlsSecrets []*controllers.TLSSecret) error {
	cfg, err := r.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	if err := upsertRecords(cfg, ing, tlsSecrets); err != nil {
		return fmt.Errorf("deleting pomerium config records: %w", err)
	}
	if err := r.saveConfig(ctx, cfg); err != nil {
		return fmt.Errorf("updating pomerium config: %w", err)
	}
	return nil
}

// Delete should delete pomerium routes corresponding to this ingress name
func (r *ConfigReconciler) Delete(ctx context.Context, namespacedName types.NamespacedName) error {
	cfg, err := r.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("get pomerium config: %w", err)
	}
	if err := deleteRecords(cfg, namespacedName); err != nil {
		return fmt.Errorf("deleting pomerium config records %s: %w", namespacedName.String(), err)
	}
	if err := r.saveConfig(ctx, cfg); err != nil {
		return fmt.Errorf("updating pomerium config: %w", err)
	}
	return nil
}

func (r *ConfigReconciler) getConfig(ctx context.Context) (*pomerium.Config, error) {
	cfg := new(pomerium.Config)
	any := protoutil.NewAny(cfg)
	var hdr metadata.MD
	resp, err := r.Get(ctx, &databroker.GetRequest{
		Type: any.GetTypeUrl(),
		Id:   configID,
	}, grpc.Header(&hdr))
	if status.Code(err) == codes.NotFound {
		return &pomerium.Config{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("get pomerium config: %w", err)
	}

	if err := resp.GetRecord().GetData().UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal current config: %w", err)
	}

	return cfg, nil
}

func (r *ConfigReconciler) saveConfig(ctx context.Context, cfg *pomerium.Config) error {
	any := protoutil.NewAny(cfg)
	if _, err := r.Put(ctx, &databroker.PutRequest{
		Record: &databroker.Record{
			Type: any.GetTypeUrl(),
			Id:   configID,
			Data: any,
		},
	}); err != nil {
		return err
	}
	return nil
}

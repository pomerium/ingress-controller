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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"
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
func (r *ConfigReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	cfg, prevBytes, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get config: %w", err)
	}

	if err = upsert(ctx, cfg, ic); err != nil {
		return false, err
	}

	return r.saveConfig(ctx, cfg, prevBytes, string(ic.Ingress.UID))
}

func (r *ConfigReconciler) Set(ctx context.Context, ics []*model.IngressConfig) error {
	logger := log.FromContext(ctx)
	cfg := new(pb.Config)

	for _, ic := range ics {
		newCfg := proto.Clone(cfg).(*pb.Config)
		if err := upsertAndValidate(ctx, newCfg, ic); err != nil {
			logger.Error(err, "skip ingress %s/%s", ic.Namespace, ic.Name)
			continue
		}
		cfg = newCfg
	}

	if _, err := r.saveConfig(ctx, cfg, nil, "config"); err != nil {
		return fmt.Errorf("saving config: %w", err)
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
	if _, err := r.saveConfig(ctx, cfg, prevBytes,
		fmt.Sprintf("%s-%s", namespacedName.Namespace, namespacedName.Name),
	); err != nil {
		return fmt.Errorf("updating pomerium config: %w", err)
	}
	return nil
}

// DeleteAll cleans pomerium configuration entirely
func (r *ConfigReconciler) DeleteAll(ctx context.Context) error {
	any := protoutil.NewAny(&pb.Config{})
	if _, err := r.Put(ctx, &databroker.PutRequest{
		Record: &databroker.Record{
			Type:      any.GetTypeUrl(),
			Id:        configID,
			Data:      any,
			DeletedAt: timestamppb.Now(),
		},
	}); err != nil {
		return err
	}
	return nil
}

func (r *ConfigReconciler) getConfig(ctx context.Context) (*pb.Config, []byte, error) {
	cfg := new(pb.Config)
	any := protoutil.NewAny(cfg)
	var hdr metadata.MD
	resp, err := r.Get(ctx, &databroker.GetRequest{
		Type: any.GetTypeUrl(),
		Id:   configID,
	}, grpc.Header(&hdr))
	if status.Code(err) == codes.NotFound {
		return &pb.Config{}, nil, nil
	} else if err != nil {
		return nil, nil, fmt.Errorf("get pomerium config: %w", err)
	}

	if err := resp.GetRecord().GetData().UnmarshalTo(cfg); err != nil {
		return nil, nil, fmt.Errorf("unmarshal current config: %w", err)
	}

	fmt.Println("<=", protojson.Format(cfg))

	return cfg, resp.GetRecord().GetData().GetValue(), nil
}

func (r *ConfigReconciler) saveConfig(ctx context.Context, cfg *pb.Config, prevBytes []byte, id string) (bool, error) {
	if err := removeUnusedCerts(cfg); err != nil {
		return false, fmt.Errorf("removing unused certs: %w", err)
	}

	any := protoutil.NewAny(cfg)
	logger := log.FromContext(ctx)

	if bytes.Equal(prevBytes, any.GetValue()) {
		logger.V(1).Info("no changes in pomerium config")
		return false, nil
	}

	if err := validate(ctx, cfg, id); err != nil {
		return false, fmt.Errorf("config validation: %w", err)
	}

	if _, err := r.Put(ctx, &databroker.PutRequest{
		Record: &databroker.Record{
			Type: any.GetTypeUrl(),
			Id:   configID,
			Data: any,
		},
	}); err != nil {
		return false, err
	}

	logger.Info("new pomerium config applied")
	// TODO: rm
	fmt.Println("=>", protojson.Format(cfg))

	return true, nil
}

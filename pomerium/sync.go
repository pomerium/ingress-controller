// Package pomerium implements logic to convert K8s objects into Pomerium configuration
package pomerium

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/hashicorp/go-multierror"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/protoutil"
	"github.com/sergi/go-diff/diffmatchpatch"

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
	DebugDumpConfigDiff bool
}

// Upsert should update or create the pomerium routes corresponding to this ingress
func (r *ConfigReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get config: %w", err)
	}

	next := proto.Clone(prev).(*pb.Config)
	if err = upsert(ctx, next, ic); err != nil {
		return false, err
	}

	return r.saveConfig(ctx, prev, next, string(ic.Ingress.UID))
}

func (r *ConfigReconciler) Set(ctx context.Context, ics []*model.IngressConfig) error {
	logger := log.FromContext(ctx)

	prev, err := r.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	next := new(pb.Config)

	for _, ic := range ics {
		cfg := proto.Clone(next).(*pb.Config)
		if err := multierror.Append(
			upsert(ctx, cfg, ic),
			validate(ctx, cfg, string(ic.Ingress.UID)),
		).ErrorOrNil(); err != nil {
			logger.Error(err, "skip ingress", "ingress", fmt.Sprintf("%s/%s", ic.Namespace, ic.Name))
			continue
		}
		next = cfg
	}

	if _, err := r.saveConfig(ctx, prev, next, "config"); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	return nil
}

// Delete should delete pomerium routes corresponding to this ingress name
func (r *ConfigReconciler) Delete(ctx context.Context, namespacedName types.NamespacedName) error {
	prev, err := r.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("get pomerium config: %w", err)
	}
	cfg := proto.Clone(prev).(*pb.Config)
	if err := deleteRoutes(ctx, cfg, namespacedName); err != nil {
		return fmt.Errorf("deleting pomerium config records %s: %w", namespacedName.String(), err)
	}
	if _, err := r.saveConfig(ctx, prev, cfg,
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

func (r *ConfigReconciler) getConfig(ctx context.Context) (*pb.Config, error) {
	cfg := new(pb.Config)
	any := protoutil.NewAny(cfg)
	var hdr metadata.MD
	resp, err := r.Get(ctx, &databroker.GetRequest{
		Type: any.GetTypeUrl(),
		Id:   configID,
	}, grpc.Header(&hdr))
	if status.Code(err) == codes.NotFound {
		return &pb.Config{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("get pomerium config: %w", err)
	}

	if err := resp.GetRecord().GetData().UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal current config: %w", err)
	}

	return cfg, nil
}

func (r *ConfigReconciler) saveConfig(ctx context.Context, prev, next *pb.Config, id string) (bool, error) {
	logger := log.FromContext(ctx)

	if err := removeUnusedCerts(next); err != nil {
		return false, fmt.Errorf("removing unused certs: %w", err)
	}
	// https://kubernetes.io/docs/concepts/services-networking/ingress/#multiple-matches
	// envoy matches according to the order routes are present in the configuration
	sort.Sort(routeList(next.Routes))

	if err := validate(ctx, next, id); err != nil {
		return false, fmt.Errorf("config validation: %w", err)
	}

	if proto.Equal(prev, next) {
		logger.Info("no changes detected")
		return false, nil
	}

	any := protoutil.NewAny(next)
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

	if r.DebugDumpConfigDiff {
		debugDumpConfigDiff(prev, next)
	}

	return true, nil
}

func debugDumpConfigDiff(prev, next *pb.Config) {
	dmp := diffmatchpatch.New()
	txt1 := protojson.Format(prev)
	txt2 := protojson.Format(next)
	diffs := dmp.DiffMain(txt1, txt2, true)
	fmt.Println("CONFIG DIFF", dmp.DiffPrettyText(diffs))
}

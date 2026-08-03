// Package pomerium implements logic to convert K8s objects into Pomerium configuration
package pomerium

import (
	"context"
	"errors"
	"fmt"

	"github.com/sergi/go-diff/diffmatchpatch"
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
	"github.com/pomerium/ingress-controller/pomerium/gateway"
)

const (
	// IngressControllerConfigID is for Ingress-defined configuration
	IngressControllerConfigID = "ingress-controller"
	// GatewayControllerConfigID is for Gateway-defined configuration
	GatewayControllerConfigID = "gateway-controller"
	// SharedSettingsConfigID is for configuration derived from the Pomerium CRD
	SharedSettingsConfigID = "pomerium-crd"
)

var (
	_ = IngressReconciler((*DataBrokerReconciler)(nil))
	_ = IngressChangeDetector((*DataBrokerReconciler)(nil))
	_ = GatewayReconciler((*DataBrokerReconciler)(nil))
	_ = ConfigReconciler((*DataBrokerReconciler)(nil))
)

// DataBrokerReconciler updates pomerium configuration
// only one DataBrokerReconciler should be active
// and its methods are not thread-safe
type DataBrokerReconciler struct {
	ConfigID string
	databroker.DataBrokerServiceClient
	// DebugDumpConfigDiff dumps a diff between current and new config being applied
	DebugDumpConfigDiff bool
	// RemoveUnreferencedCerts would strip any certs not matched by any of the Routes SNI
	RemoveUnreferencedCerts bool
	// ingressConfigCache avoids rebuilding the monolithic Pomerium config for
	// duplicate Kubernetes events. The zero value is ready for use.
	ingressConfigCache ingressConfigCache
}

// NeedsIngressUpdate reports whether the current Kubernetes inputs differ
// from the last configuration successfully persisted by this reconciler.
func (r *DataBrokerReconciler) NeedsIngressUpdate(ic *model.IngressConfig) (bool, error) {
	fingerprint, err := fingerprintIngressConfig(ic)
	if err != nil {
		return false, fmt.Errorf("fingerprint ingress config: %w", err)
	}
	return !r.ingressConfigCache.hit(ic.GetIngressNamespacedName(), fingerprint), nil
}

// TracksIngress reports whether this reconciler last persisted an Ingress.
func (r *DataBrokerReconciler) TracksIngress(namespacedName types.NamespacedName) bool {
	return r.ingressConfigCache.contains(namespacedName)
}

// Upsert should update or create the pomerium routes corresponding to this ingress
func (r *DataBrokerReconciler) Upsert(ctx context.Context, ic *model.IngressConfig) (bool, error) {
	fingerprint, err := fingerprintIngressConfig(ic)
	if err != nil {
		return false, fmt.Errorf("fingerprint ingress config: %w", err)
	}
	if r.ingressConfigCache.hit(ic.GetIngressNamespacedName(), fingerprint) {
		return false, nil
	}

	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get config: %w", err)
	}

	next := proto.Clone(prev).(*pb.Config)
	if err = upsertRoutes(ctx, next, ic); err != nil {
		return false, err
	}
	addCerts(next, ic.Secrets)

	changed, err := r.saveConfig(ctx, prev, next, fmt.Sprintf("%s-%s", r.ConfigID, ic.Ingress.UID))
	if err == nil {
		r.ingressConfigCache.store(ic.GetIngressNamespacedName(), fingerprint)
	}
	return changed, err
}

// Set merges existing config with the one generated for ingress
func (r *DataBrokerReconciler) Set(ctx context.Context, ics []*model.IngressConfig) (bool, error) {
	fingerprints, err := fingerprintIngressConfigs(ics)
	if err != nil {
		return false, fmt.Errorf("fingerprint ingress configs: %w", err)
	}
	if r.ingressConfigCache.matches(fingerprints) {
		return false, nil
	}

	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get config: %w", err)
	}
	next := buildIngressConfig(ctx, ics, nil)

	changed, err := r.saveConfig(ctx, prev, next, r.ConfigID)
	var validationErr *configValidationError
	if errors.As(err, &validationErr) {
		// Keep the fast path to one full validation. If it fails, fall back to
		// validating each Ingress in isolation so one invalid object still does
		// not block unrelated valid changes.
		next = buildIngressConfig(ctx, ics, validate)
		changed, err = r.saveConfig(ctx, prev, next, r.ConfigID)
	}
	if err == nil {
		r.ingressConfigCache.replace(fingerprints)
	}
	return changed, err
}

type configValidator func(context.Context, *pb.Config, string) error

// buildIngressConfig builds each Ingress in isolation before merging it into
// the full configuration. The optional validator is used by the slow fallback
// after full validation fails. Cloning and validating the accumulated config
// for every Ingress makes reconciliation quadratic and can leave removed
// endpoints active while a large batch is being rebuilt.
func buildIngressConfig(
	ctx context.Context,
	ics []*model.IngressConfig,
	validator configValidator,
) *pb.Config {
	logger := log.FromContext(ctx)
	next := new(pb.Config)

	for _, ic := range ics {
		cfg := new(pb.Config)
		if err := upsertRoutes(ctx, cfg, ic); err != nil {
			logger.Error(err, "skip ingress", "ingress", fmt.Sprintf("%s/%s", ic.Namespace, ic.Name))
			continue
		}
		if validator != nil {
			if err := validator(ctx, cfg, string(ic.Ingress.UID)); err != nil {
				logger.Error(err, "skip ingress", "ingress", fmt.Sprintf("%s/%s", ic.Namespace, ic.Name))
				continue
			}
		}
		next.Routes = append(next.Routes, cfg.Routes...)
		addCerts(next, ic.Secrets)
	}

	return next
}

// SetConfig updates just the shared config settings
func (r *DataBrokerReconciler) SetConfig(ctx context.Context, cfg *model.Config) (changes bool, err error) {
	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get config: %w", err)
	}
	next := new(pb.Config)

	if err = ApplyConfig(ctx, next, cfg); err != nil {
		return false, fmt.Errorf("settings: %w", err)
	}

	return r.saveConfig(ctx, prev, next, r.ConfigID)
}

// Delete should delete pomerium routes corresponding to this ingress name
func (r *DataBrokerReconciler) Delete(ctx context.Context, namespacedName types.NamespacedName) (bool, error) {
	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get pomerium config: %w", err)
	}
	cfg := proto.Clone(prev).(*pb.Config)
	if err := deleteRoutes(cfg, namespacedName); err != nil {
		return false, fmt.Errorf("deleting pomerium config records %s: %w", namespacedName.String(), err)
	}
	changed, err := r.saveConfig(ctx, prev, cfg, fmt.Sprintf("%s-%s", namespacedName.Namespace, namespacedName.Name))
	if err != nil {
		return false, fmt.Errorf("updating pomerium config: %w", err)
	}
	r.ingressConfigCache.delete(namespacedName)
	return changed, nil
}

// SetGatewayConfig applies Gateway-defined configuration.
func (r *DataBrokerReconciler) SetGatewayConfig(
	ctx context.Context,
	config *model.GatewayConfig,
) (changes bool, err error) {
	prev, err := r.getConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get config: %w", err)
	}
	next := new(pb.Config)

	for i := range config.Routes {
		r := &config.Routes[i]
		next.Routes = append(next.Routes, gateway.TranslateRoutes(ctx, config, r)...)
	}
	next.Settings = new(pb.Settings)
	for _, cert := range config.Certificates {
		addTLSCert(next.Settings, cert)
	}

	return r.saveConfig(ctx, prev, next, r.ConfigID)
}

// DeleteAll cleans pomerium configuration entirely
func (r *DataBrokerReconciler) DeleteAll(ctx context.Context) error {
	data := protoutil.NewAny(&pb.Config{})
	if _, err := r.Put(ctx, &databroker.PutRequest{
		Records: []*databroker.Record{{
			Type:      data.GetTypeUrl(),
			Id:        IngressControllerConfigID,
			Data:      data,
			DeletedAt: timestamppb.Now(),
		}},
	}); err != nil {
		return err
	}
	return nil
}

func (r *DataBrokerReconciler) getConfig(ctx context.Context) (*pb.Config, error) {
	cfg := new(pb.Config)
	data := protoutil.NewAny(cfg)
	var hdr metadata.MD
	resp, err := r.Get(ctx, &databroker.GetRequest{
		Type: data.GetTypeUrl(),
		Id:   r.ConfigID,
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

func (r *DataBrokerReconciler) saveConfig(ctx context.Context, prev, next *pb.Config, id string) (bool, error) {
	if r.RemoveUnreferencedCerts {
		if err := removeUnusedCerts(next); err != nil {
			return false, fmt.Errorf("removing unused certs: %w", err)
		}
	}

	ensureDeterministicConfigOrder(next)

	if err := validate(ctx, next, id); err != nil {
		return false, &configValidationError{err: err}
	}

	logger := log.FromContext(ctx)
	if proto.Equal(prev, next) {
		logger.V(1).Info("no changes in the config")
		return false, nil
	}

	data := protoutil.NewAny(next)
	if _, err := r.Put(ctx, &databroker.PutRequest{
		Records: []*databroker.Record{{
			Type: data.GetTypeUrl(),
			Id:   r.ConfigID,
			Data: data,
		}},
	}); err != nil {
		return false, err
	}

	if r.DebugDumpConfigDiff {
		logger.Info("config diff", "diff", debugDumpConfigDiff(prev, next))
	}
	logger.Info("new pomerium config applied")

	return true, nil
}

type configValidationError struct {
	err error
}

func (e *configValidationError) Error() string {
	return fmt.Sprintf("config validation: %v", e.err)
}

func (e *configValidationError) Unwrap() error {
	return e.err
}

func debugDumpConfigDiff(prev, next *pb.Config) []byte {
	dmp := diffmatchpatch.New()
	txt1 := protojson.Format(prev)
	txt2 := protojson.Format(next)
	diffs := dmp.DiffMain(txt1, txt2, true)
	return []byte(dmp.DiffPrettyText(diffs))
}

package certificate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pomerium_ingress_v1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/internal/certificate"
	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
	databrokerpb "github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"
)

type recordKey struct {
	Type string
	ID   string
}

// A dataBrokerCollector collects data from the databroker to determine which
// routes need certificates provisioned.
type dataBrokerCollector struct {
	controller *certificateController
	operation  Operation

	keyPairServerVersion, keyPairRecordVersion                 uint64
	routeServerVersion, routeRecordVersion                     uint64
	settingsServerVersion, settingsRecordVersion               uint64
	configServerVersion, configRecordVersion                   uint64
	versionedConfigServerVersion, versionedConfigRecordVersion uint64

	mu      sync.Mutex
	matcher certificate.Matcher[recordKey]
}

func newDataBrokerCollector(controller *certificateController) *dataBrokerCollector {
	c := &dataBrokerCollector{
		controller: controller,
		operation:  NewOperation(),
	}
	return c
}

// MissingNames returns the list of missing names from the matcher.
func (c *dataBrokerCollector) MissingNames() []string {
	c.mu.Lock()
	missingNames := c.matcher.MissingNames()
	c.mu.Unlock()
	return missingNames
}

// Stop will stop any background sync processes.
func (c *dataBrokerCollector) Stop() {
	_ = c.operation.Stop()
}

// Sync syncs data from the databroker. If no data has been synced successfully
// yet, an initial sync latest will be done and any errors returned. Background
// sync processes will be started that can detect changes to databroker objects
// and call reconcile on the controller.
func (c *dataBrokerCollector) Sync() error {
	// If the previous sync operation returned an error, return that. The
	// controller reconciliation loop should call sync again next time so
	// we reset the operation to re-initialize.
	if err := c.operation.Error(); err != nil {
		c.operation.Reset()
		c.mu.Lock()
		c.matcher = nil
		c.mu.Unlock()
		return err
	}

	// if no matcher is defined yet, run init and wait for it to complete
	c.mu.Lock()
	if c.matcher == nil {
		c.operation.Start(c.init)
		c.mu.Unlock()
		err := c.operation.Wait()
		if err != nil {
			c.operation.Reset()
			return err
		}
	} else {
		c.mu.Unlock()
	}

	// start the background sync
	if !c.operation.Active() {
		c.operation.Start(c.sync)
	}

	return nil
}

func (c *dataBrokerCollector) init(ctx context.Context) error {
	c.mu.Lock()
	c.matcher = certificate.NewMatcher[recordKey]()
	c.mu.Unlock()

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		c.keyPairServerVersion, c.keyPairRecordVersion, err = syncLatestRecords(ctx, c.controller.dataBrokerClient,
			func(record *databrokerpb.Record, keyPair *configpb.KeyPair) {
				certificateNames, routeNames := certificate.GetNamesFromConfig([]*configpb.KeyPair{keyPair}, nil, nil)
				c.mu.Lock()
				c.matcher.Update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
				c.mu.Unlock()
			})
		if err != nil {
			return fmt.Errorf("error syncing latest key pairs: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		var err error
		c.routeServerVersion, c.routeRecordVersion, err = syncLatestRecords(ctx, c.controller.dataBrokerClient,
			func(record *databrokerpb.Record, route *configpb.Route) {
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, []*configpb.Route{route}, nil)
				c.mu.Lock()
				c.matcher.Update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
				c.mu.Unlock()
			})
		if err != nil {
			return fmt.Errorf("error syncing latest routes: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		var err error
		c.settingsServerVersion, c.settingsRecordVersion, err = syncLatestRecords(ctx, c.controller.dataBrokerClient,
			func(record *databrokerpb.Record, settings *configpb.Settings) {
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, nil, []*configpb.Settings{settings})
				c.mu.Lock()
				c.matcher.Update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
				c.mu.Unlock()
			})
		if err != nil {
			return fmt.Errorf("error syncing latest settings: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		var err error
		c.configServerVersion, c.configRecordVersion, err = syncLatestRecords(ctx, c.controller.dataBrokerClient,
			func(record *databrokerpb.Record, config *configpb.Config) {
				// ignore the config we create
				if record.GetId() == dataBrokerConfigRecordID {
					return
				}
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, config.Routes, []*configpb.Settings{config.Settings})
				c.mu.Lock()
				c.matcher.Update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
				c.mu.Unlock()
			})
		if err != nil {
			return fmt.Errorf("error syncing latest configs: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		var err error
		c.versionedConfigServerVersion, c.versionedConfigRecordVersion, err = syncLatestRecords(ctx, c.controller.dataBrokerClient,
			func(record *databrokerpb.Record, versionedConfig *configpb.VersionedConfig) {
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, versionedConfig.Config.Routes, []*configpb.Settings{versionedConfig.Config.Settings})
				c.mu.Lock()
				c.matcher.Update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
				c.mu.Unlock()
			})
		if err != nil {
			return fmt.Errorf("error syncing latest versioned configs: %w", err)
		}
		return nil
	})
	return eg.Wait()
}

func (c *dataBrokerCollector) sync(ctx context.Context) error {
	update := func(key recordKey, certificateNames, routeNames []string) {
		log.FromContext(ctx).Info("certificate-controller: databroker record updated",
			"record-type", key.Type,
			"record-id", key.ID)
		c.mu.Lock()
		c.matcher.Update(key, certificateNames, routeNames)
		c.mu.Unlock()

		if err := c.controller.kubernetesClient.Status().Patch(ctx, &pomerium_ingress_v1.Pomerium{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: c.controller.globalSettingsName.Namespace,
				Name:      c.controller.globalSettingsName.Name,
			},
			Status: pomerium_ingress_v1.PomeriumStatus{
				CertificateAutoProvisionStatus: &pomerium_ingress_v1.CertificateAutoProvisionStatus{
					DataBrokerLastUpdated: meta_v1.Now(),
				},
			},
		}, client.MergeFrom(&pomerium_ingress_v1.Pomerium{ObjectMeta: meta_v1.ObjectMeta{
			Namespace: c.controller.globalSettingsName.Namespace,
			Name:      c.controller.globalSettingsName.Name,
		}})); err != nil {
			log.FromContext(ctx).Error(err, "error creating event")
		}
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		err := syncRecords(ctx, c.controller.dataBrokerClient,
			c.keyPairServerVersion, c.keyPairRecordVersion,
			func(record *databrokerpb.Record, keyPair *configpb.KeyPair) {
				certificateNames, routeNames := certificate.GetNamesFromConfig([]*configpb.KeyPair{keyPair}, nil, nil)
				update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
			})
		if err != nil {
			return fmt.Errorf("error syncing key pairs: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		err := syncRecords(ctx, c.controller.dataBrokerClient,
			c.routeServerVersion, c.routeRecordVersion,
			func(record *databrokerpb.Record, route *configpb.Route) {
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, []*configpb.Route{route}, nil)
				update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
			})
		if err != nil {
			return fmt.Errorf("error syncing routes: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		err := syncRecords(ctx, c.controller.dataBrokerClient,
			c.settingsServerVersion, c.settingsRecordVersion,
			func(record *databrokerpb.Record, settings *configpb.Settings) {
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, nil, []*configpb.Settings{settings})
				update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
			})
		if err != nil {
			return fmt.Errorf("error syncing settings: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		err := syncRecords(ctx, c.controller.dataBrokerClient,
			c.configServerVersion, c.configRecordVersion,
			func(record *databrokerpb.Record, config *configpb.Config) {
				// ignore the config we create
				if record.GetId() == dataBrokerConfigRecordID {
					return
				}
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, config.Routes, []*configpb.Settings{config.Settings})
				update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
			})
		if err != nil {
			return fmt.Errorf("error syncing configs: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		err := syncRecords(ctx, c.controller.dataBrokerClient,
			c.versionedConfigServerVersion, c.versionedConfigRecordVersion,
			func(record *databrokerpb.Record, versionedConfig *configpb.VersionedConfig) {
				certificateNames, routeNames := certificate.GetNamesFromConfig(nil, versionedConfig.Config.Routes, []*configpb.Settings{versionedConfig.Config.Settings})
				update(recordKey{Type: record.GetType(), ID: record.GetId()}, certificateNames, routeNames)
			})
		if err != nil {
			return fmt.Errorf("error syncing configs: %w", err)
		}
		return nil
	})
	return eg.Wait()
}

func syncRecords[T any, TMsg interface {
	*T
	proto.Message
}](
	ctx context.Context,
	client databrokerpb.DataBrokerServiceClient,
	serverVersion, recordVersion uint64,
	fn func(record *databrokerpb.Record, object TMsg),
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	recordType := grpcutil.GetTypeURL(TMsg(new(T)))
	res, err := client.Sync(ctx, &databrokerpb.SyncRequest{
		Type:          recordType,
		ServerVersion: serverVersion,
		RecordVersion: recordVersion,
	})
	if err != nil {
		return fmt.Errorf("error syncing %s records: %w", recordType, err)
	}

	for {
		msg, err := res.Recv()
		if errors.Is(err, io.EOF) {
			break
		} else if status.Code(err) == codes.Aborted {
			return fmt.Errorf("aborted sync due to mismatched versions: %w", err)
		} else if err != nil {
			return fmt.Errorf("error receiving %s record: %w", recordType, err)
		}

		switch res := msg.Response.(type) {
		case *databrokerpb.SyncResponse_Record:
			recordVersion = max(recordVersion, res.Record.Version)
			var m T
			// for deleted records just use an empty protobuf
			if res.Record.GetDeletedAt() == nil {
				if err := res.Record.Data.UnmarshalTo(TMsg(&m)); err != nil {
					continue
				}
			}
			fn(res.Record, TMsg(&m))
		}
	}

	return io.ErrUnexpectedEOF
}

func syncLatestRecords[T any, TMsg interface {
	*T
	proto.Message
}](
	ctx context.Context,
	client databrokerpb.DataBrokerServiceClient,
	fn func(record *databrokerpb.Record, object TMsg),
) (serverVersion, recordVersion uint64, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	recordType := grpcutil.GetTypeURL(TMsg(new(T)))
	res, err := client.SyncLatest(ctx, &databrokerpb.SyncLatestRequest{
		Type: recordType,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("error syncing latest %s records: %w", recordType, err)
	}

	for {
		msg, err := res.Recv()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return 0, 0, fmt.Errorf("error receiving latest %s record: %w", recordType, err)
		}

		switch res := msg.Response.(type) {
		case *databrokerpb.SyncLatestResponse_Record:
			var m T
			if err := res.Record.GetData().UnmarshalTo(TMsg(&m)); err != nil {
				continue
			}
			fn(res.Record, TMsg(&m))
		case *databrokerpb.SyncLatestResponse_Versions:
			serverVersion = res.Versions.GetServerVersion()
			recordVersion = res.Versions.GetLatestRecordVersion()
		}
	}

	return serverVersion, recordVersion, err
}

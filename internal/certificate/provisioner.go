package certificate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cenkalti/backoff/v4"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"

	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
	databrokerpb "github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpcutil"
)

var reinitialize = errors.New("reinitialize")

type recordKey struct {
	Type string
	ID   string
}

// A Provisioner provisions missing certificates.
type Provisioner struct {
	dataBrokerClient   databrokerpb.DataBrokerServiceClient
	kubernetesClient   client.WithWatch
	globalSettingsName types.NamespacedName

	matcher Matcher[recordKey]

	enabled                                                    bool
	clusterIssuer                                              string
	keyPairServerVersion, keyPairRecordVersion                 uint64
	routeServerVersion, routeRecordVersion                     uint64
	settingsServerVersion, settingsRecordVersion               uint64
	configServerVersion, configRecordVersion                   uint64
	versionedConfigServerVersion, versionedConfigRecordVersion uint64
}

// NewProvisioner creates a new Provisioner.
func NewProvisioner(
	dataBrokerClient databrokerpb.DataBrokerServiceClient,
	kubernetesClient client.WithWatch,
	globalSettingsName types.NamespacedName,
) *Provisioner {
	return &Provisioner{
		dataBrokerClient:   dataBrokerClient,
		kubernetesClient:   kubernetesClient,
		globalSettingsName: globalSettingsName,

		matcher: NewMatcher[recordKey](),
	}
}

// Run runs the provisioner.
func (p *Provisioner) Run(ctx context.Context) error {
	b := backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(0))
	return backoff.RetryNotify(
		func() error {
			err := p.init(ctx)
			if err != nil {
				return fmt.Errorf("error initializing data: %w", err)
			}

			b.Reset()

			err = p.sync(ctx)
			if err != nil {
				return fmt.Errorf("error syncing data: %w", err)
			}

			return nil
		},
		backoff.WithContext(b, ctx),
		func(err error, next time.Duration) {
			log.FromContext(ctx).Error(err, "certificate-provisioner: error while running", "next", next)
		},
	)
}

func (p *Provisioner) init(ctx context.Context) error {
	log.FromContext(ctx).Info("certificate-provisioner: initializing")

	if err := p.initSettings(ctx); err != nil {
		return err
	}

	if err := p.initCertificates(ctx); err != nil {
		return err
	}

	if err := p.initDataBrokerData(ctx); err != nil {
		return err
	}

	return nil
}

func (p *Provisioner) initCertificates(ctx context.Context) error {
	if !p.enabled {
		return nil
	}

	var l certmanager.CertificateList
	if err := p.kubernetesClient.List(ctx, &l, client.MatchingLabels{
		"app.kubernetes.io/managed-by": "pomerium",
	}); err != nil {
		return fmt.Errorf("error listing kubernetes certificates: %w", err)
	}
	log.FromContext(ctx).Info("certificate-provisioner: found certificates", "certificates", l.Items)

	return nil
}

func (p *Provisioner) initDataBrokerData(ctx context.Context) error {
	p.matcher = NewMatcher[recordKey]()

	if !p.enabled {
		return nil
	}

	var err error

	p.keyPairServerVersion, p.keyPairRecordVersion, err = syncLatest(ctx, p.dataBrokerClient,
		func(record *databrokerpb.Record, keyPair *configpb.KeyPair) {
			certificateNames, routeNames := GetNamesFromConfig([]*configpb.KeyPair{keyPair}, nil, nil)
			p.matcher.Update(recordKey{
				Type: record.GetType(),
				ID:   record.GetId(),
			}, certificateNames, routeNames)
		})
	if err != nil {
		return fmt.Errorf("error syncing latest key pairs: %w", err)
	}

	p.routeServerVersion, p.routeRecordVersion, err = syncLatest(ctx, p.dataBrokerClient,
		func(record *databrokerpb.Record, route *configpb.Route) {
			certificateNames, routeNames := GetNamesFromConfig(nil, []*configpb.Route{route}, nil)
			p.matcher.Update(recordKey{
				Type: record.GetType(),
				ID:   record.GetId(),
			}, certificateNames, routeNames)
		})
	if err != nil {
		return fmt.Errorf("error syncing latest routes: %w", err)
	}

	p.settingsServerVersion, p.settingsRecordVersion, err = syncLatest(ctx, p.dataBrokerClient,
		func(record *databrokerpb.Record, settings *configpb.Settings) {
			certificateNames, routeNames := GetNamesFromConfig(nil, nil, []*configpb.Settings{settings})
			p.matcher.Update(recordKey{
				Type: record.GetType(),
				ID:   record.GetId(),
			}, certificateNames, routeNames)
		})
	if err != nil {
		return fmt.Errorf("error syncing latest settings: %w", err)
	}

	p.configServerVersion, p.configRecordVersion, err = syncLatest(ctx, p.dataBrokerClient,
		func(record *databrokerpb.Record, cfg *configpb.Config) {
			certificateNames, routeNames := GetNamesFromConfig(nil, cfg.Routes, []*configpb.Settings{cfg.Settings})
			p.matcher.Update(recordKey{
				Type: record.GetType(),
				ID:   record.GetId(),
			}, certificateNames, routeNames)
		})
	if err != nil {
		return fmt.Errorf("error syncing latest configs: %w", err)
	}

	p.versionedConfigServerVersion, p.versionedConfigRecordVersion, err = syncLatest(ctx, p.dataBrokerClient,
		func(record *databrokerpb.Record, cfg *configpb.VersionedConfig) {
			certificateNames, routeNames := GetNamesFromConfig(nil, cfg.Config.Routes, []*configpb.Settings{cfg.Config.Settings})
			p.matcher.Update(recordKey{
				Type: record.GetType(),
				ID:   record.GetId(),
			}, certificateNames, routeNames)
		})
	if err != nil {
		return fmt.Errorf("error syncing latest versioned configs: %w", err)
	}

	return nil
}

func (p *Provisioner) initSettings(ctx context.Context) error {
	p.enabled = false
	var obj icsv1.Pomerium
	err := client.IgnoreNotFound(p.kubernetesClient.Get(ctx, p.globalSettingsName, &obj))
	if err != nil {
		return fmt.Errorf("error retrieving pomerium settings: %w", err)
	}
	if obj.Spec.CertificateAutoProvision != nil && obj.Spec.CertificateAutoProvision.ClusterIssuer != nil {
		p.enabled = true
		p.clusterIssuer = *obj.Spec.CertificateAutoProvision.ClusterIssuer
	}
	return nil
}

func (p *Provisioner) sync(ctx context.Context) error {
	log.FromContext(ctx).Info("certificate-provisioner: syncing")

	type Update struct {
		key              recordKey
		certificateNames []string
		routeNames       []string
	}
	updateCh := make(chan Update, 1)

	type MissingNames []string
	missingNamesCh := make(chan MissingNames, 1)
	missingNamesCh <- p.matcher.MissingNames()

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return p.syncSettings(ctx)
	})

	// sync databroker data
	eg.Go(func() error {
		return sync(ctx, p.dataBrokerClient, p.keyPairServerVersion, p.keyPairRecordVersion,
			func(record *databrokerpb.Record, keyPair *configpb.KeyPair) {
				certificateNames, routeNames := GetNamesFromConfig([]*configpb.KeyPair{keyPair}, nil, nil)
				select {
				case updateCh <- Update{
					key:              recordKey{Type: record.GetType(), ID: record.GetId()},
					certificateNames: certificateNames,
					routeNames:       routeNames,
				}:
				case <-ctx.Done():
				}
			})
	})
	eg.Go(func() error {
		return sync(ctx, p.dataBrokerClient, p.routeServerVersion, p.routeRecordVersion,
			func(record *databrokerpb.Record, route *configpb.Route) {
				certificateNames, routeNames := GetNamesFromConfig(nil, []*configpb.Route{route}, nil)
				select {
				case updateCh <- Update{
					key:              recordKey{Type: record.GetType(), ID: record.GetId()},
					certificateNames: certificateNames,
					routeNames:       routeNames,
				}:
				case <-ctx.Done():
				}
			})
	})
	eg.Go(func() error {
		return sync(ctx, p.dataBrokerClient, p.settingsServerVersion, p.settingsRecordVersion,
			func(record *databrokerpb.Record, settings *configpb.Settings) {
				certificateNames, routeNames := GetNamesFromConfig(nil, nil, []*configpb.Settings{settings})
				select {
				case updateCh <- Update{
					key:              recordKey{Type: record.GetType(), ID: record.GetId()},
					certificateNames: certificateNames,
					routeNames:       routeNames,
				}:
				case <-ctx.Done():
				}
			})
	})
	eg.Go(func() error {
		return sync(ctx, p.dataBrokerClient, p.configServerVersion, p.configRecordVersion,
			func(record *databrokerpb.Record, cfg *configpb.Config) {
				certificateNames, routeNames := GetNamesFromConfig(nil, cfg.Routes, []*configpb.Settings{cfg.Settings})
				select {
				case updateCh <- Update{
					key:              recordKey{Type: record.GetType(), ID: record.GetId()},
					certificateNames: certificateNames,
					routeNames:       routeNames,
				}:
				case <-ctx.Done():
				}
			})
	})
	eg.Go(func() error {
		return sync(ctx, p.dataBrokerClient, p.configServerVersion, p.configRecordVersion,
			func(record *databrokerpb.Record, cfg *configpb.VersionedConfig) {
				certificateNames, routeNames := GetNamesFromConfig(nil, cfg.Config.Routes, []*configpb.Settings{cfg.Config.Settings})
				select {
				case updateCh <- Update{
					key:              recordKey{Type: record.GetType(), ID: record.GetId()},
					certificateNames: certificateNames,
					routeNames:       routeNames,
				}:
				case <-ctx.Done():
				}
			})
	})

	// collect updates
	eg.Go(func() error {
		for {
			var update Update
			select {
			case update = <-updateCh:
			case <-ctx.Done():
				return context.Cause(ctx)
			}

			p.matcher.Update(update.key, update.certificateNames, update.routeNames)

			select {
			case missingNamesCh <- p.matcher.MissingNames():
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		}
	})

	// process missing names
	eg.Go(func() error {
		for {
			var missingNames MissingNames
			select {
			case missingNames = <-missingNamesCh:
			case <-ctx.Done():
				return context.Cause(ctx)
			}

			log.FromContext(ctx).Info("certificate-provisioner: processing missing names", "missing-names", missingNames)
		}
	})

	return eg.Wait()
}

func (p *Provisioner) syncSettings(ctx context.Context) error {
	w, err := p.kubernetesClient.Watch(ctx, new(icsv1.PomeriumList))
	if err != nil {
		return fmt.Errorf("error watching pomerium settings: %w", err)
	}
	defer w.Stop()

	for {
		select {
		case _, ok := <-w.ResultChan():
			if !ok {
				return io.ErrUnexpectedEOF
			}
			var obj icsv1.Pomerium
			err := client.IgnoreNotFound(p.kubernetesClient.Get(ctx, p.globalSettingsName, &obj))
			if err != nil {
				return fmt.Errorf("error retrieving pomerium settings: %w", err)
			}

			prevEnabled, prevClusterIssuer := p.enabled, p.clusterIssuer
			nextEnabled, nextClusterIssuer := prevEnabled, prevClusterIssuer
			if obj.Spec.CertificateAutoProvision != nil && obj.Spec.CertificateAutoProvision.ClusterIssuer != nil {
				nextEnabled, nextClusterIssuer = true, *obj.Spec.CertificateAutoProvision.ClusterIssuer
			} else if p.enabled {
				nextEnabled, nextClusterIssuer = false, ""
			}

			if prevEnabled != nextEnabled || prevClusterIssuer != nextClusterIssuer {
				return reinitialize
			}
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

func sync[T any, TMsg interface {
	*T
	proto.Message
}](
	ctx context.Context,
	client databrokerpb.DataBrokerServiceClient,
	serverVersion, recordVersion uint64,
	fn func(record *databrokerpb.Record, object TMsg),
) error {
	return backoff.RetryNotify(
		func() error {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			recordType := grpcutil.GetTypeURL(TMsg(new(T)))
			res, err := client.Sync(ctx, &databrokerpb.SyncRequest{
				Type:          recordType,
				ServerVersion: serverVersion,
				RecordVersion: recordVersion,
			})
			if err != nil {
				return fmt.Errorf("error syncing %s records", recordType)
			}

			for {
				msg, err := res.Recv()
				if errors.Is(err, io.EOF) {
					break
				} else if status.Code(err) == codes.Aborted {
					return backoff.Permanent(fmt.Errorf("aborted sync due to mismatched versions: %w", err))
				} else if err != nil {
					return fmt.Errorf("error receiving %s record", recordType)
				}

				switch res := msg.Response.(type) {
				case *databrokerpb.SyncResponse_Record:
					recordVersion = max(recordVersion, res.Record.Version)
					var m T
					err := res.Record.Data.UnmarshalTo(TMsg(&m))
					if err == nil {
						fn(res.Record, TMsg(&m))
					}
				}
			}

			return io.ErrUnexpectedEOF
		},
		backoff.WithContext(backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(0)), ctx),
		func(err error, next time.Duration) {
			log.FromContext(ctx).Error(err, "certificate-provisioner: error while syncing", "next", next)
		},
	)
}

func syncLatest[T any, TMsg interface {
	*T
	proto.Message
}](
	ctx context.Context,
	client databrokerpb.DataBrokerServiceClient,
	fn func(record *databrokerpb.Record, object TMsg),
) (serverVersion, recordVersion uint64, err error) {
	err = backoff.RetryNotify(
		func() error {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			recordType := grpcutil.GetTypeURL(TMsg(new(T)))
			res, err := client.SyncLatest(ctx, &databrokerpb.SyncLatestRequest{
				Type: recordType,
			})
			if err != nil {
				return fmt.Errorf("error syncing latest %s records", recordType)
			}

			for {
				msg, err := res.Recv()
				if errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					return fmt.Errorf("error receiving latest %s record", recordType)
				}

				switch res := msg.Response.(type) {
				case *databrokerpb.SyncLatestResponse_Record:
					var m T
					err := res.Record.Data.UnmarshalTo(TMsg(&m))
					if err == nil {
						fn(res.Record, TMsg(&m))
					}
				case *databrokerpb.SyncLatestResponse_Versions:
					serverVersion = res.Versions.GetServerVersion()
					recordVersion = res.Versions.GetLatestRecordVersion()
				}
			}

			return nil
		},
		backoff.WithContext(backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(0)), ctx),
		func(err error, next time.Duration) {
			log.FromContext(ctx).Error(err, "certificate-provisioner: error while syncing latest", "next", next)
		},
	)
	return serverVersion, recordVersion, err
}

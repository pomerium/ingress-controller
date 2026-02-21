package pomerium

import (
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/sdk-go"
)

// NewDataBrokerReconcilers returns a set of reconcilers that use the databroker API.
func NewDataBrokerReconcilers(
	client databroker.DataBrokerServiceClient,
	dumpConfigDiff bool,
) (IngressReconciler, ConfigReconciler, GatewayReconciler) {
	return &DataBrokerReconciler{
			ConfigID:                IngressControllerConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     dumpConfigDiff,
			RemoveUnreferencedCerts: true,
		},
		&DataBrokerReconciler{
			ConfigID:                SharedSettingsConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     dumpConfigDiff,
			RemoveUnreferencedCerts: false,
		},
		&DataBrokerReconciler{
			ConfigID:                GatewayControllerConfigID,
			DataBrokerServiceClient: client,
			DebugDumpConfigDiff:     dumpConfigDiff,
			RemoveUnreferencedCerts: false,
		}
}

func NewUnifiedAPIReconcilers(
	client sdk.Client,
) (IngressReconciler, ConfigReconciler, GatewayReconciler) {
	r := &APIReconciler{client: client}
	return r, r, r
}

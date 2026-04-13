// Package mock_test contains mock clients for testing
package mock_test

//go:generate go run go.uber.org/mock/mockgen -package mock_test -destination client.go sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate go run go.uber.org/mock/mockgen -package mock_test -destination pomerium_ingress_reconciler.go github.com/pomerium/ingress-controller/pomerium IngressReconciler
//go:generate go run go.uber.org/mock/mockgen -package mock_test -destination pomerium_config_reconciler.go github.com/pomerium/ingress-controller/pomerium ConfigReconciler
//go:generate go run go.uber.org/mock/mockgen -package mock_test -destination sdkclient.go -mock_names Client=MockSDKClient github.com/pomerium/sdk-go Client

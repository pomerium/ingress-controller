module github.com/pomerium/ingress-controller

go 1.16

require (
	github.com/envoyproxy/go-control-plane v0.9.9 // indirect
	github.com/onsi/ginkgo v1.16.1
	github.com/onsi/gomega v1.11.0
	github.com/pomerium/pomerium v0.15.1-0.20210810012516-e38682d02460
	github.com/spf13/cobra v1.2.1
	go.uber.org/zap v1.19.0 // indirect
	google.golang.org/grpc v1.39.1
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)

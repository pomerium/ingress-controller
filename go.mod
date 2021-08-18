module github.com/pomerium/ingress-controller

go 1.16

require (
	github.com/envoyproxy/go-control-plane v0.9.9 // indirect
	github.com/gosimple/slug v1.10.0
	github.com/jetstack/cert-manager v1.5.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.14.0
	github.com/pomerium/pomerium v0.15.1-0.20210810012516-e38682d02460
	github.com/spf13/cobra v1.2.1
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.19.0
	google.golang.org/grpc v1.39.1
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	sigs.k8s.io/controller-runtime v0.9.2
)

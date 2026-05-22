package cmd

import (
	"github.com/spf13/pflag"

	"github.com/pomerium/ingress-controller/controllers/certificate"
)

type certificateControllerOptions struct {
	Name string
}

func (o *certificateControllerOptions) setupFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.Name, "certificate-controller-name", certificate.DefaultControllerName,
		"the name of the certificate controller")
}

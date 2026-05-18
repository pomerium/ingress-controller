package cmd

import "github.com/spf13/pflag"

type certificateControllerOptions struct {
	Name string
}

func (o *certificateControllerOptions) setupFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.Name, "certificate-controller-name", "pomerium-certificate",
		"the name of the certificate controller")
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	stress_cmd "github.com/pomerium/ingress-controller/internal/stress/cmd"
)

// RootCommand generates default secrets
func RootCommand() (*cobra.Command, error) {
	root := cobra.Command{
		Use:          "ingress-controller",
		Short:        "pomerium ingress controller",
		SilenceUsage: true,
	}

	for name, fn := range map[string]func() (*cobra.Command, error){
		"gen-secrets": GenSecretsCommand,
		"controller":  ControllerCommand,
		"all-in-one":  AllInOneCommand,
		"stress-test": stress_cmd.Command,
	} {
		cmd, err := fn()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		root.AddCommand(cmd)
	}

	return &root, nil
}

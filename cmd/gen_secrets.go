package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	runtime_ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pomerium/ingress-controller/util"
)

type genSecretsCmd struct {
	secrets string
	debug   bool

	cobra.Command
}

// GenSecretsCommand generates default secrets
func GenSecretsCommand() (*cobra.Command, error) {
	cmd := genSecretsCmd{
		Command: cobra.Command{
			Use:   "gen-secrets",
			Short: "generates default secrets",
		}}
	cmd.RunE = cmd.exec
	if err := cmd.setupFlags(); err != nil {
		return nil, err
	}
	return &cmd.Command, nil
}

func (s *genSecretsCmd) setupFlags() error {
	flags := s.PersistentFlags()
	flags.BoolVar(&s.debug, debug, false, "enable debug logging")
	if err := flags.MarkHidden("debug"); err != nil {
		return err
	}
	flags.StringVar(&s.secrets, "secrets", "", "namespaced name of a Secret object to generate")

	v := viper.New()
	var err error
	flags.VisitAll(func(f *pflag.Flag) {
		if err = v.BindEnv(f.Name, envName(f.Name)); err != nil {
			return
		}

		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			if err = flags.Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				return
			}
		}
	})
	return err
}

func (s *genSecretsCmd) exec(*cobra.Command, []string) error {
	setupLogger(s.debug)
	ctx := runtime_ctrl.SetupSignalHandler()

	name, err := util.ParseNamespacedName(s.secrets)
	if err != nil {
		return fmt.Errorf("%s=%s: %w", globalSettings, s.secrets, err)
	}

	cfg, err := runtime_ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get k8s api config: %w", err)
	}

	scheme, err := getScheme()
	if err != nil {
		return fmt.Errorf("scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}

	secret, err := util.NewBootstrapSecrets(*name)
	if err != nil {
		return fmt.Errorf("generate secrets: %w", err)
	}

	return c.Create(ctx, secret)
}

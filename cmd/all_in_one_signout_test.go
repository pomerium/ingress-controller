package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
	pomeriumctrl "github.com/pomerium/ingress-controller/pomerium/ctrl"
	"github.com/pomerium/ingress-controller/util"
	"github.com/pomerium/pomerium/config"
)

func TestAllInOneSignOutRedirect(t *testing.T) {
	t.Setenv("SIGNOUT_REDIRECT_URL", "")
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "default"},
		{name: "https", args: []string{"--signout-redirect-url=https://site.example/"}, want: "https://site.example/"},
		{name: "local http", args: []string{"--signout-redirect-url=http://localhost:8080/logged-out"}, want: "http://localhost:8080/logged-out"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := allCmd{}
			require.NoError(t, cmd.setupFlags())
			require.NoError(t, cmd.PersistentFlags().Parse(append([]string{"--pomerium-config=global"}, tt.args...)))
			require.NoError(t, cmd.Validate())
			var param allCmdParam
			require.NoError(t, param.makeBootstrapConfig(t.Context(), cmd.allCmdOptions))
			assert.Equal(t, tt.want, param.cfg.Options.SignOutRedirectURLString)

			// CRD reconciliation must not discard the command's bootstrap setting.
			secrets, err := util.NewBootstrapSecrets(types.NamespacedName{})
			require.NoError(t, err)
			runner, err := pomeriumctrl.NewPomeriumRunner(param.cfg, func(context.Context, *config.Config) {}, false)
			require.NoError(t, err)
			_, err = runner.SetConfig(t.Context(), &model.Config{Secrets: secrets})
			require.NoError(t, err)
			assert.Equal(t, tt.want, runner.GetConfig().Options.SignOutRedirectURLString)
		})
	}
}

func TestAllInOneSignOutRedirectEnvironment(t *testing.T) {
	t.Setenv("SIGNOUT_REDIRECT_URL", "https://site.example/logged-out")
	cmd := allCmd{}
	require.NoError(t, cmd.setupFlags())
	assert.Equal(t, "https://site.example/logged-out", cmd.signOutRedirectURL)
	require.NoError(t, cmd.PersistentFlags().Parse([]string{"--signout-redirect-url=https://other.example/"}))
	assert.Equal(t, "https://other.example/", cmd.signOutRedirectURL)
}

func TestAllInOneSignOutRedirectValidation(t *testing.T) {
	for _, value := range []string{"/logged-out", "site.example", "javascript:alert(1)", "ftp://site.example/", "https://", "https://user:password@site.example/", "https://site.example/%invalid"} {
		t.Run(value, func(t *testing.T) {
			cmd := allCmd{}
			require.NoError(t, cmd.setupFlags())
			require.NoError(t, cmd.PersistentFlags().Parse([]string{"--pomerium-config=global"}))
			require.NoError(t, cmd.Validate())
			require.NoError(t, cmd.PersistentFlags().Parse([]string{"--signout-redirect-url=" + value}))
			require.Error(t, cmd.Validate())
		})
	}
}

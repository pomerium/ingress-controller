package pomerium

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"

	"google.golang.org/protobuf/types/known/durationpb"
	corev1 "k8s.io/api/core/v1"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
)

func applyConfig(p *pb.Config, c *model.Config) error {
	if c == nil {
		return nil
	}

	if p.Settings == nil {
		p.Settings = new(pb.Settings)
	}

	for _, apply := range []struct {
		name string
		fn   func(*pb.Config, *model.Config) error
	}{
		{"certs", applyCerts},
		{"authenticate", applyAuthenticate},
		{"idp", applyIDP},
		{"idp url", applyIDPProviderURL},
		{"idp refresh", applyIDPProviderRefreshTimeouts},
		{"idp secret", applyIDPSecret},
		{"idp service account from secret", applyServiceAccount},
		{"idp request params", applyIDPRequestParams},
	} {
		if err := apply.fn(p, c); err != nil {
			return fmt.Errorf("%s: %w", apply.name, err)
		}
	}

	return nil
}

func applyCerts(p *pb.Config, c *model.Config) error {
	if len(c.Certs) != len(c.Spec.Certificates) {
		return fmt.Errorf("expected %d cert secrets, only %d was fetched. this is a bug", len(c.Spec.Certificates), len(c.Certs))
	}

	for _, secret := range c.Certs {
		if secret.Type != corev1.SecretTypeTLS {
			return fmt.Errorf("%s expected secret type %s, got %s", util.GetNamespacedName(secret), corev1.SecretTypeTLS, secret.Type)
		}
		addTLSCert(p.Settings, secret)
	}
	return nil
}

func applyAuthenticate(p *pb.Config, c *model.Config) error {
	_, err := url.Parse(c.Spec.Authenticate.URL)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", c.Spec.Authenticate.URL, err)
	}
	p.Settings.AuthenticateServiceUrl = &c.Spec.Authenticate.URL
	p.Settings.AuthenticateCallbackPath = c.Spec.Authenticate.CallbackPath

	return nil
}

func applyIDP(p *pb.Config, c *model.Config) error {
	idp := c.Spec.IdentityProvider
	p.Settings.IdpProvider = &idp.Provider
	p.Settings.Scopes = idp.Scopes

	return nil
}

func applyIDPProviderRefreshTimeouts(p *pb.Config, c *model.Config) error {
	rd := c.Pomerium.Spec.IdentityProvider.RefreshDirectory
	if rd == nil {
		return nil
	}
	p.Settings.IdpRefreshDirectoryInterval = durationpb.New(rd.Interval.Duration)
	p.Settings.IdpRefreshDirectoryTimeout = durationpb.New(rd.Timeout.Duration)

	return nil
}

func applyIDPProviderURL(p *pb.Config, c *model.Config) error {
	if c.Spec.IdentityProvider.URL == nil {
		return nil
	}

	if _, err := url.Parse(*c.Spec.IdentityProvider.URL); err != nil {
		return err
	}
	p.Settings.IdpProviderUrl = c.Spec.IdentityProvider.URL

	return nil
}

func applyIDPSecret(p *pb.Config, c *model.Config) error {
	if c.IdpSecret == nil {
		return fmt.Errorf("is required")
	}

	var err error
	if p.Settings.IdpClientId, err = getRequiredString(c.IdpSecret.Data, "client_id"); err != nil {
		return err
	}
	if p.Settings.IdpClientSecret, err = getRequiredString(c.IdpSecret.Data, "client_secret"); err != nil {
		return err
	}

	if data, ok := c.IdpSecret.Data["service_account"]; ok {
		txt := string(data)
		p.Settings.IdpServiceAccount = &txt
	}

	return nil
}

func applyIDPRequestParams(p *pb.Config, c *model.Config) error {
	if c.RequestParams == nil {
		p.Settings.RequestParams = c.Spec.IdentityProvider.RequestParams
		return nil
	}
	var err error
	p.Settings.RequestParams, err = util.MergeMaps(c.Spec.IdentityProvider.RequestParams, c.RequestParams.Data)
	if err != nil {
		return err
	}
	return nil
}

func applyServiceAccount(p *pb.Config, c *model.Config) error {
	if c.IdpServiceAccount == nil {
		return nil
	}
	if p.Settings.IdpServiceAccount != nil {
		return fmt.Errorf("service account was already set from secret %s", c.Spec.IdentityProvider.Secret)
	}
	txt, err := buildIDPServiceAccount(c.IdpServiceAccount)
	if err != nil {
		return err
	}
	p.Settings.IdpServiceAccount = &txt
	return nil
}

// buildIDPServiceAccount builds an IdP service account from a provided secret keys
// see https://www.pomerium.com/reference/#identity-provider-service-account
func buildIDPServiceAccount(secret *corev1.Secret) (string, error) {
	sm := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		sm[k] = string(v)
	}
	data, err := json.Marshal(sm)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func getRequiredString(data map[string][]byte, key string) (*string, error) {
	bs, ok := data[key]
	if !ok || len(bs) == 0 {
		return nil, fmt.Errorf("%s key is required", key)
	}
	txt := string(bs)
	return &txt, nil
}

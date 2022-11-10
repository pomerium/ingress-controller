package pomerium

import (
	"context"
	"fmt"
	"net/url"

	"google.golang.org/protobuf/types/known/durationpb"
	corev1 "k8s.io/api/core/v1"

	"github.com/pomerium/pomerium/config"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
)

func applyConfig(ctx context.Context, p *pb.Config, c *model.Config) error {
	if c == nil {
		return nil
	}

	if p.Settings == nil {
		p.Settings = new(pb.Settings)
	}

	for _, apply := range []struct {
		name string
		fn   func(context.Context, *pb.Config, *model.Config) error
	}{
		{"certs", applyCerts},
		{"authenticate", applyAuthenticate},
		{"idp", applyIDP},
		{"idp url", applyIDPProviderURL},
		{"idp secret", applyIDPSecret},
		{"idp request params", applyIDPRequestParams},
		{"cookie", applyCookie},
		{"jwt claim headers", applyJWTClaimHeaders},
	} {
		if err := apply.fn(ctx, p, c); err != nil {
			return fmt.Errorf("%s: %w", apply.name, err)
		}
	}

	return nil
}

func applyJWTClaimHeaders(_ context.Context, p *pb.Config, c *model.Config) error {
	p.Settings.JwtClaimsHeaders = c.Spec.JWTClaimHeaders
	return nil
}

func applyCookie(_ context.Context, p *pb.Config, c *model.Config) error {
	if c.Spec.Cookie == nil {
		return nil
	}
	p.Settings.CookieDomain = c.Spec.Cookie.Domain
	p.Settings.CookieName = c.Spec.Cookie.Name
	p.Settings.CookieHttpOnly = c.Spec.Cookie.HTTPOnly
	p.Settings.CookieSecure = c.Spec.Cookie.Secure

	if c.Spec.Cookie.Expire != nil {
		p.Settings.CookieExpire = durationpb.New(c.Spec.Cookie.Expire.Duration)
	}

	return nil
}

func applyCerts(_ context.Context, p *pb.Config, c *model.Config) error {
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

func applyAuthenticate(_ context.Context, p *pb.Config, c *model.Config) error {
	_, err := url.Parse(c.Spec.Authenticate.URL)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", c.Spec.Authenticate.URL, err)
	}
	p.Settings.AuthenticateServiceUrl = &c.Spec.Authenticate.URL
	p.Settings.AuthenticateCallbackPath = c.Spec.Authenticate.CallbackPath

	return nil
}

func applyIDP(_ context.Context, p *pb.Config, c *model.Config) error {
	idp := c.Spec.IdentityProvider
	p.Settings.IdpProvider = &idp.Provider
	p.Settings.Scopes = idp.Scopes

	return nil
}

func applyIDPProviderURL(_ context.Context, p *pb.Config, c *model.Config) error {
	if c.Spec.IdentityProvider.URL == nil {
		return nil
	}

	if _, err := url.Parse(*c.Spec.IdentityProvider.URL); err != nil {
		return err
	}
	p.Settings.IdpProviderUrl = c.Spec.IdentityProvider.URL

	return nil
}

func applyIDPSecret(ctx context.Context, p *pb.Config, c *model.Config) error {
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

	if _, ok := c.IdpSecret.Data["service_account"]; ok {
		util.Add[config.FieldMsg](ctx, config.FieldMsg{
			Key:           "identityProvider.secret.service_account",
			DocsURL:       "https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync",
			FieldCheckMsg: config.FieldCheckMsgRemoved,
			KeyAction:     config.KeyActionWarn,
		})
	}

	return nil
}

func applyIDPRequestParams(_ context.Context, p *pb.Config, c *model.Config) error {
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

func getRequiredString(data map[string][]byte, key string) (*string, error) {
	bs, ok := data[key]
	if !ok || len(bs) == 0 {
		return nil, fmt.Errorf("%s key is required", key)
	}
	txt := string(bs)
	return &txt, nil
}

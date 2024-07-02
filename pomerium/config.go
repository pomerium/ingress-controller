package pomerium

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	corev1 "k8s.io/api/core/v1"

	"github.com/pomerium/pomerium/config"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
)

type applyOpt struct {
	name string
	fn   func(context.Context, *pb.Config, *model.Config) error
}

func applyConfig(ctx context.Context, p *pb.Config, c *model.Config) error {
	if c == nil {
		return nil
	}

	if p.Settings == nil {
		p.Settings = new(pb.Settings)
	}

	opts := []applyOpt{
		{"ca", applyCertificateAuthority},
		{"client ca", applyClientCertificate},
		{"certs", applyCerts},
		{"authenticate", applyAuthenticate},
		{"cookie", applyCookie},
		{"warnings", checkForWarnings},
		{"jwt claim headers", applyJWTClaimHeaders},
		{"timeouts", applyTimeouts},
		{"misc opts", applySetOtherOptions},
	}
	if c.Spec.IdentityProvider != nil {
		opts = append(opts, []applyOpt{
			{"idp", applyIDP},
			{"idp url", applyIDPProviderURL},
			{"idp secret", applyIDPSecret},
			{"idp request params", applyIDPRequestParams},
		}...)
	}

	for _, apply := range opts {
		if err := apply.fn(ctx, p, c); err != nil {
			return fmt.Errorf("%s: %w", apply.name, err)
		}
	}

	return nil
}

func checkForWarnings(ctx context.Context, _ *pb.Config, c *model.Config) error {
	if c.Spec.Storage == nil || c.Spec.Storage.Postgres == nil {
		util.Add(ctx, config.FieldMsg{
			Key:           "storage",
			DocsURL:       "https://www.pomerium.com/docs/topics/data-storage#persistence",
			FieldCheckMsg: "please specify a persistent storage backend",
			KeyAction:     config.KeyActionWarn,
		})
	}
	return nil
}

func applyTimeouts(_ context.Context, p *pb.Config, c *model.Config) error {
	if c.Spec.Timeouts == nil {
		return nil
	}
	tm := c.Spec.Timeouts

	if tm.Read != nil && tm.Write != nil && tm.Read.Duration >= tm.Write.Duration {
		return fmt.Errorf("read timeout (%s) must be less than write timeout (%s)", tm.Read.Duration, tm.Write.Duration)
	}
	if tm.Idle != nil {
		p.Settings.TimeoutIdle = durationpb.New(tm.Idle.Duration)
	}
	if tm.Read != nil {
		p.Settings.TimeoutRead = durationpb.New(tm.Read.Duration)
	}
	if tm.Write != nil {
		p.Settings.TimeoutWrite = durationpb.New(tm.Write.Duration)
	}

	return nil
}

func applyJWTClaimHeaders(_ context.Context, p *pb.Config, c *model.Config) error {
	p.Settings.JwtClaimsHeaders = c.Spec.JWTClaimHeaders
	return nil
}

func applySetOtherOptions(_ context.Context, p *pb.Config, c *model.Config) error {
	p.Settings.SetResponseHeaders = c.Spec.SetResponseHeaders
	p.Settings.ProgrammaticRedirectDomainWhitelist = c.Spec.ProgrammaticRedirectDomains
	p.Settings.UseProxyProtocol = c.Spec.UseProxyProtocol
	if c.Spec.AccessLogFields != nil {
		p.Settings.AccessLogFields = &pb.Settings_StringList{
			Values: *c.Spec.AccessLogFields,
		}
	}
	if c.Spec.AuthorizeLogFields != nil {
		p.Settings.AuthorizeLogFields = &pb.Settings_StringList{
			Values: *c.Spec.AuthorizeLogFields,
		}
	}
	if c.Spec.PassIdentityHeaders != nil {
		p.Settings.PassIdentityHeaders = proto.Bool(*c.Spec.PassIdentityHeaders)
	} else {
		p.Settings.PassIdentityHeaders = nil
	}
	return nil
}

func applyCookie(_ context.Context, p *pb.Config, c *model.Config) error {
	if c.Spec.Cookie == nil {
		return nil
	}
	p.Settings.CookieDomain = c.Spec.Cookie.Domain
	p.Settings.CookieName = c.Spec.Cookie.Name
	p.Settings.CookieHttpOnly = c.Spec.Cookie.HTTPOnly
	if c.Spec.Cookie.Expire != nil {
		p.Settings.CookieExpire = durationpb.New(c.Spec.Cookie.Expire.Duration)
	}
	p.Settings.CookieSameSite = c.Spec.Cookie.SameSite

	return nil
}

func applyCertificateAuthority(_ context.Context, p *pb.Config, c *model.Config) error {
	if len(c.CASecrets) == 0 {
		return nil
	}

	var buf bytes.Buffer

	for _, secret := range c.CASecrets {
		buf.Write(secret.Data[model.CAKey])
		buf.WriteRune('\n')
	}

	p.Settings.CertificateAuthority = proto.String(base64.StdEncoding.EncodeToString(buf.Bytes()))
	return nil
}

func applyClientCertificate(_ context.Context, p *pb.Config, c *model.Config) error {
	if len(c.ClientCASecrets) == 0 {
		return nil
	}

	var crtBuf bytes.Buffer
	var crlBuf bytes.Buffer

	for _, secret := range c.ClientCASecrets {
		crtBuf.Write(secret.Data[model.CAKey])
		crtBuf.WriteRune('\n')
		crlBuf.Write(secret.Data[model.CRLKey])
		crlBuf.WriteRune('\n')
	}

	p.Settings.ClientCa = proto.String(base64.StdEncoding.EncodeToString(crtBuf.Bytes()))
	p.Settings.ClientCrl = proto.String(base64.StdEncoding.EncodeToString(crlBuf.Bytes()))
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
	if c.Spec.Authenticate == nil {
		p.Settings.AuthenticateServiceUrl = proto.String("https://authenticate.pomerium.app")
		p.Settings.AuthenticateCallbackPath = proto.String("/oauth2/callback")
		return nil
	}

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
		util.Add(ctx, config.FieldMsg{
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

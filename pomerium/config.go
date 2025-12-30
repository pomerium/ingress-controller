package pomerium

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	corev1 "k8s.io/api/core/v1"

	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
	"github.com/pomerium/pomerium/config"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

type applyOpt struct {
	name string
	fn   func(context.Context, *pb.Config, *model.Config) error
}

// ApplyConfig applies an ingress controller model config to a protobuf destination.
func ApplyConfig(ctx context.Context, dst *pb.Config, src *model.Config) error {
	if src == nil {
		return nil
	}

	if dst.Settings == nil {
		dst.Settings = new(pb.Settings)
	}

	opts := []applyOpt{
		{"ca", applyCertificateAuthority},
		{"certs", applyCerts},
		{"authenticate", applyAuthenticate},
		{"cookie", applyCookie},
		{"warnings", checkForWarnings},
		{"jwt claim headers", applyJWTClaimHeaders},
		{"timeouts", applyTimeouts},
		{"misc opts", applySetOtherOptions},
		{"otel", applyOTEL},
		{"downstream mtls", applyDownstreamMTLS},
		{"circuit breaker thresholds", applyCircuitBreakerThresholds},
		{"dns", applyDNS},
		{"ssh", applySSH},
	}
	if src.Spec.IdentityProvider != nil {
		opts = append(opts, []applyOpt{
			{"idp", applyIDP},
			{"idp url", applyIDPProviderURL},
			{"idp secret", applyIDPSecret},
			{"idp request params", applyIDPRequestParams},
		}...)
	}

	for _, apply := range opts {
		if err := apply.fn(ctx, dst, src); err != nil {
			return fmt.Errorf("%s: %w", apply.name, err)
		}
	}

	return nil
}

func checkForWarnings(ctx context.Context, _ *pb.Config, c *model.Config) error {
	if c.Spec.Storage == nil || (c.Spec.Storage.File == nil && c.Spec.Storage.Postgres == nil) {
		util.Add(ctx, config.FieldMsg{
			Key:           "storage",
			DocsURL:       "https://www.pomerium.com/docs/internals/data-storage",
			FieldCheckMsg: "please specify a persistent storage backend",
			KeyAction:     config.KeyActionWarn,
		})
	}
	return nil
}

func applyOTEL(_ context.Context, p *pb.Config, c *model.Config) error {
	otel := c.Spec.OTEL
	if otel == nil {
		return nil
	}

	otlp := "otlp"
	p.Settings.OtelTracesExporter = &otlp

	_, err := url.Parse(otel.Endpoint)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", otel.Endpoint, err)
	}
	p.Settings.OtelExporterOtlpTracesEndpoint = &otel.Endpoint

	var sampling *float64
	if otel.Sampling != nil {
		v, err := strconv.ParseFloat(*otel.Sampling, 64)
		if err != nil {
			return fmt.Errorf("invalid sampling value %s: %w", *otel.Sampling, err)
		}
		if v < 0 || v > 1 {
			return fmt.Errorf("sampling value %f must be in [0:1] range", v)
		}
		sampling = &v
	}
	p.Settings.OtelTracesSamplerArg = sampling

	p.Settings.OtelResourceAttributes = mapToKVSlice(otel.ResourceAttributes)
	p.Settings.OtelLogLevel = otel.LogLevel
	p.Settings.OtelExporterOtlpTracesProtocol = &otel.Protocol

	p.Settings.OtelExporterOtlpTracesHeaders = mapToKVSlice(otel.Headers)

	if otel.Timeout != nil {
		p.Settings.OtelExporterOtlpTracesTimeout = durationpb.New(otel.Timeout.Duration)
	}
	if otel.BSPScheduleDelay != nil {
		p.Settings.OtelBspScheduleDelay = durationpb.New(otel.BSPScheduleDelay.Duration)
	}
	if otel.BSPMaxExportBatchSize != nil {
		p.Settings.OtelBspMaxExportBatchSize = otel.BSPMaxExportBatchSize
	}

	return nil
}

// mapToKVSlice converts a map to a slice of key=value strings.
func mapToKVSlice(m map[string]string) []string {
	var res []string
	for k, v := range m {
		res = append(res, fmt.Sprintf("%s=%s", k, v))
	}
	return res
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
	if c.Spec.CodecType != nil {
		switch *c.Spec.CodecType {
		case "auto":
			p.Settings.CodecType = pb.CodecType_CODEC_TYPE_AUTO.Enum()
		case "http1":
			p.Settings.CodecType = pb.CodecType_CODEC_TYPE_HTTP1.Enum()
		case "http2":
			p.Settings.CodecType = pb.CodecType_CODEC_TYPE_HTTP2.Enum()
		case "http3":
			p.Settings.CodecType = pb.CodecType_CODEC_TYPE_HTTP3.Enum()
		default:
			return fmt.Errorf("unknown codecType %s", *c.Spec.CodecType)
		}
	}
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
	if c.Spec.BearerTokenFormat != nil {
		switch *c.Spec.BearerTokenFormat {
		case "":
			p.Settings.BearerTokenFormat = pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_UNKNOWN.Enum()
		case "default":
			p.Settings.BearerTokenFormat = pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_DEFAULT.Enum()
		case "idp_access_token":
			p.Settings.BearerTokenFormat = pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_IDP_ACCESS_TOKEN.Enum()
		case "idp_identity_token":
			p.Settings.BearerTokenFormat = pb.BearerTokenFormat_BEARER_TOKEN_FORMAT_IDP_IDENTITY_TOKEN.Enum()
		default:
			return fmt.Errorf("unknown bearerTokenFormat %s", *c.Spec.BearerTokenFormat)
		}
	} else {
		p.Settings.BearerTokenFormat = nil
	}
	if c.Spec.IDPAccessTokenAllowedAudiences != nil {
		p.Settings.IdpAccessTokenAllowedAudiences = &pb.Settings_StringList{
			Values: *c.Spec.IDPAccessTokenAllowedAudiences,
		}
	} else {
		p.Settings.IdpAccessTokenAllowedAudiences = nil
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

func applyCerts(_ context.Context, p *pb.Config, c *model.Config) error {
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
		return nil
	}

	_, err := url.Parse(c.Spec.Authenticate.URL)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", c.Spec.Authenticate.URL, err)
	}
	p.Settings.AuthenticateServiceUrl = &c.Spec.Authenticate.URL

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

func applyDownstreamMTLS(_ context.Context, dst *pb.Config, src *model.Config) error {
	// nothing to do
	if src.Spec.DownstreamMTLS == nil {
		dst.Settings.DownstreamMtls = nil
		return nil
	}

	dst.Settings.DownstreamMtls = new(pb.DownstreamMtlsSettings)

	if len(src.Spec.DownstreamMTLS.CA) > 0 {
		dst.Settings.DownstreamMtls.Ca = proto.String(base64.StdEncoding.EncodeToString(src.Spec.DownstreamMTLS.CA))
	}
	if len(src.Spec.DownstreamMTLS.CRL) > 0 {
		dst.Settings.DownstreamMtls.Crl = proto.String(base64.StdEncoding.EncodeToString(src.Spec.DownstreamMTLS.CRL))
	}
	if src.Spec.DownstreamMTLS.Enforcement != nil {
		switch strings.ToLower(*src.Spec.DownstreamMTLS.Enforcement) {
		case "policy_with_default_deny":
			dst.Settings.DownstreamMtls.Enforcement = pb.MtlsEnforcementMode_POLICY_WITH_DEFAULT_DENY.Enum()
		case "policy":
			dst.Settings.DownstreamMtls.Enforcement = pb.MtlsEnforcementMode_POLICY.Enum()
		case "reject_connection":
			dst.Settings.DownstreamMtls.Enforcement = pb.MtlsEnforcementMode_REJECT_CONNECTION.Enum()
		}
	}
	if src.Spec.DownstreamMTLS.MatchSubjectAltNames != nil {
		m := map[pb.SANMatcher_SANType]string{
			pb.SANMatcher_EMAIL:               src.Spec.DownstreamMTLS.MatchSubjectAltNames.Email,
			pb.SANMatcher_DNS:                 src.Spec.DownstreamMTLS.MatchSubjectAltNames.DNS,
			pb.SANMatcher_URI:                 src.Spec.DownstreamMTLS.MatchSubjectAltNames.URI,
			pb.SANMatcher_IP_ADDRESS:          src.Spec.DownstreamMTLS.MatchSubjectAltNames.IPAddress,
			pb.SANMatcher_USER_PRINCIPAL_NAME: src.Spec.DownstreamMTLS.MatchSubjectAltNames.UserPrincipalName,
		}
		for k, v := range m {
			if v == "" {
				continue
			}
			dst.Settings.DownstreamMtls.MatchSubjectAltNames = append(dst.Settings.DownstreamMtls.MatchSubjectAltNames, &pb.SANMatcher{
				SanType: k,
				Pattern: v,
			})
		}
	}
	if src.Spec.DownstreamMTLS.MaxVerifyDepth != nil {
		dst.Settings.DownstreamMtls.MaxVerifyDepth = proto.Uint32(*src.Spec.DownstreamMTLS.MaxVerifyDepth)
	}

	return nil
}

func applyCircuitBreakerThresholds(_ context.Context, dst *pb.Config, src *model.Config) error {
	if src.Spec.CircuitBreakerThresholds == nil {
		dst.Settings.CircuitBreakerThresholds = nil
		return nil
	}

	dst.Settings.CircuitBreakerThresholds = &pb.CircuitBreakerThresholds{
		MaxConnections:     src.Spec.CircuitBreakerThresholds.MaxConnections,
		MaxPendingRequests: src.Spec.CircuitBreakerThresholds.MaxPendingRequests,
		MaxRequests:        src.Spec.CircuitBreakerThresholds.MaxRequests,
		MaxRetries:         src.Spec.CircuitBreakerThresholds.MaxRetries,
		MaxConnectionPools: src.Spec.CircuitBreakerThresholds.MaxConnectionPools,
	}
	return nil
}

func applyDNS(_ context.Context, dst *pb.Config, src *model.Config) error {
	if src.Spec.DNS == nil {
		dst.Settings.DnsLookupFamily = nil
		dst.Settings.DnsFailureRefreshRate = nil
		dst.Settings.DnsQueryTimeout = nil
		dst.Settings.DnsQueryTries = nil
		dst.Settings.DnsRefreshRate = nil
		dst.Settings.DnsUdpMaxQueries = nil
		dst.Settings.DnsUseTcp = nil
		return nil
	}

	if src.Spec.DNS.LookupFamily != nil {
		dst.Settings.DnsLookupFamily = proto.String(strings.ToUpper(*src.Spec.DNS.LookupFamily))
	} else {
		dst.Settings.DnsLookupFamily = nil
	}
	if src.Spec.DNS.FailureRefreshRate != nil {
		dst.Settings.DnsFailureRefreshRate = durationpb.New(src.Spec.DNS.FailureRefreshRate.Duration)
	} else {
		dst.Settings.DnsFailureRefreshRate = nil
	}
	if src.Spec.DNS.QueryTimeout != nil {
		dst.Settings.DnsQueryTimeout = durationpb.New(src.Spec.DNS.QueryTimeout.Duration)
	} else {
		dst.Settings.DnsQueryTimeout = nil
	}
	dst.Settings.DnsQueryTries = src.Spec.DNS.QueryTries
	if src.Spec.DNS.RefreshRate != nil {
		dst.Settings.DnsRefreshRate = durationpb.New(src.Spec.DNS.RefreshRate.Duration)
	} else {
		dst.Settings.DnsRefreshRate = nil
	}
	dst.Settings.DnsUdpMaxQueries = src.Spec.DNS.UDPMaxQueries
	dst.Settings.DnsUseTcp = src.Spec.DNS.UseTCP
	return nil
}

func applySSH(_ context.Context, dst *pb.Config, src *model.Config) error {
	if secrets := src.SSHSecrets.HostKeys; len(secrets) > 0 {
		dst.Settings.SshHostKeys = &pb.Settings_StringList{Values: make([]string, 0, len(secrets))}
		for _, secret := range secrets {
			data, ok := secret.Data[model.SSHPrivateKey]
			if !ok {
				return fmt.Errorf("missing ssh host key data in %s", util.GetNamespacedName(secret))
			}
			dst.Settings.SshHostKeys.Values = append(dst.Settings.SshHostKeys.Values, string(data))
		}
	} else {
		dst.Settings.SshHostKeys = nil
	}

	if secret := src.SSHSecrets.UserCAKey; secret != nil {
		data, ok := secret.Data[model.SSHPrivateKey]
		if !ok {
			return fmt.Errorf("missing ssh user ca key data in %s", util.GetNamespacedName(secret))
		}
		dst.Settings.SshUserCaKey = proto.String(string(data))
	} else {
		dst.Settings.SshUserCaKey = nil
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

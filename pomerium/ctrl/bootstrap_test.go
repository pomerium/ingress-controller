package ctrl

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/pomerium/config"
	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/nullable"

	icsv1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/util"
)

func mustB64Decode(t *testing.T, txt string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(txt)
	require.NoError(t, err)
	return data
}

func TestSecretsDecode(t *testing.T) {
	secrets, err := util.NewBootstrapSecrets(types.NamespacedName{})
	require.NoError(t, err)

	var opts config.Options
	require.NoError(t, applySecrets(context.Background(), &opts, &model.Config{Secrets: secrets}))

	assert.Equal(t, base64.StdEncoding.EncodeToString(secrets.Data["cookie_secret"]), opts.CookieSecret)
	assert.Equal(t, base64.StdEncoding.EncodeToString(secrets.Data["shared_secret"]), opts.SharedKey)
	assert.Equal(t, base64.StdEncoding.EncodeToString(secrets.Data["signing_key"]), opts.SigningKey)
}

func TestSecretsDecodeRules(t *testing.T) {
	var opts config.Options

	assert.NoError(t, applySecrets(context.Background(), &opts, &model.Config{
		Secrets: &v1.Secret{
			Data: map[string][]byte{
				"shared_secret": mustB64Decode(t, "9OkZR6hwfmVD3a7Sfmgq58lUbFJGGz4hl/R9xbHFCAg="),
				"cookie_secret": mustB64Decode(t, "WwMtDXWaRDMBQCylle8OJ+w4kLIDIGd8W3cB4/zFFtg="),
				"signing_key":   mustB64Decode(t, "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUhQbkN5MXk0TEZZVkhQb3RzM05rUSttTXJLcDgvVmVWRkRwaUk2TVNxMlVvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFT1h0VXAxOWFwRnNvVWJoYkI2cExMR1o1WFBXRlE5YWtmeW5ISy9RZ3paNC9MRjZhWEY2egpvS3lHMnNtL2wyajFiQ1JxUGJNd3dEVW9iWFNIODVIeDdRPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),
			},
		},
	}), "ok secret")

	assert.Error(t, applySecrets(context.Background(), &opts, &model.Config{}))
	assert.Error(t, applySecrets(context.Background(), &opts, &model.Config{
		Secrets: &v1.Secret{
			Data: map[string][]byte{
				"cookie_secret": mustB64Decode(t, "WwMtDXWaRDMBQCylle8OJ+w4kLIDIGd8W3cB4/zFFtg="),
				"signing_key":   mustB64Decode(t, "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUhQbkN5MXk0TEZZVkhQb3RzM05rUSttTXJLcDgvVmVWRkRwaUk2TVNxMlVvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFT1h0VXAxOWFwRnNvVWJoYkI2cExMR1o1WFBXRlE5YWtmeW5ISy9RZ3paNC9MRjZhWEY2egpvS3lHMnNtL2wyajFiQ1JxUGJNd3dEVW9iWFNIODVIeDdRPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),
			},
		},
	}))
	assert.Error(t, applySecrets(context.Background(), &opts, &model.Config{
		Secrets: &v1.Secret{
			Data: map[string][]byte{
				"shared_secret": {1, 2, 3},
				"cookie_secret": mustB64Decode(t, "WwMtDXWaRDMBQCylle8OJ+w4kLIDIGd8W3cB4/zFFtg="),
				"signing_key":   mustB64Decode(t, "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUhQbkN5MXk0TEZZVkhQb3RzM05rUSttTXJLcDgvVmVWRkRwaUk2TVNxMlVvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFT1h0VXAxOWFwRnNvVWJoYkI2cExMR1o1WFBXRlE5YWtmeW5ISy9RZ3paNC9MRjZhWEY2egpvS3lHMnNtL2wyajFiQ1JxUGJNd3dEVW9iWFNIODVIeDdRPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),
			},
		},
	}))
}

const exampleCert = `-----BEGIN CERTIFICATE-----
MIIBXzCCAQagAwIBAgICEAAwCgYIKoZIzj0EAwIwFzEVMBMGA1UEAxMMVGVzdCBS
b290IENBMB4XDTI1MDkxMjE1NTk1M1oXDTM1MDkxMDE1NTk1M1owFzEVMBMGA1UE
AxMMVGVzdCBSb290IENBMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEnomv0HAN
5+BEp3H5LZPl7WE3KWa6VPAxBpCf8BXYpyaJH2PG7VJ1Ateu/I2Y/+AH4f8m6DHV
iHhi3ll2Qu1oLqNCMEAwDgYDVR0PAQH/BAQDAgEGMA8GA1UdEwEB/wQFMAMBAf8w
HQYDVR0OBBYEFEibW9kXYo7F3BR++6c8lMlU5GAYMAoGCCqGSM49BAMCA0cAMEQC
IDopBqx9I9AxBtHFzAESP7uoReoRdwwoqdUDY+I+/kW0AiAY1wV3V3A4fSdjV6x9
fk5EbQ+E27ez9yyDZ6XtQKlJLQ==
-----END CERTIFICATE-----`

func TestApplyAdditional(t *testing.T) {
	// ApplyAdditional propagates additional core bootstrap settings from the
	// Pomerium CRD spec to the config.Options struct.
	idpURL := "https://idp.example.com"
	cfg := &model.Config{
		Pomerium: icsv1.Pomerium{
			Spec: icsv1.PomeriumSpec{
				Authenticate: &icsv1.Authenticate{
					URL: "https://authenticate.example.com",
				},
				IdentityProvider: &icsv1.IdentityProvider{
					Provider: "oidc",
					URL:      &idpURL,
					Secret:   "test/idp-client-secret",
				},
			},
		},
		IdpSecret: &v1.Secret{
			Data: map[string][]byte{
				"client_id":     []byte("test-client-id"),
				"client_secret": []byte("test-client-secret"),
			},
		},
		Certs: map[types.NamespacedName]*v1.Secret{
			{Namespace: "test", Name: "my-cert"}: {
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(exampleCert),
					"tls.key": []byte("not-a-real-key"),
				},
			},
		},
	}

	var opts config.Options
	err := ApplyAdditional(t.Context(), &opts, cfg)
	require.NoError(t, err)
	assert.Equal(t, "https://authenticate.example.com", opts.AuthenticateURLString)
	assert.Equal(t, "oidc", opts.Provider)
	assert.Equal(t, "https://idp.example.com", opts.ProviderURL)
	assert.Equal(t, "test-client-id", opts.ClientID)
	assert.Equal(t, "test-client-secret", opts.ClientSecret)
	if assert.Len(t, opts.CertificateData, 1) {
		assert.Equal(t, []byte(exampleCert), opts.CertificateData[0].CertBytes)
		assert.Equal(t, []byte("not-a-real-key"), opts.CertificateData[0].KeyBytes)
	}
}

func TestConvertEnterprise(t *testing.T) {
	t.Parallel()

	dst := new(config.EnterpriseSettings)
	src := &icsv1.Enterprise{
		Administrators:                       new([]string{"u1", "u2", "u3"}),
		AnalyticsCollectInterval:             new(metav1.Duration{Duration: time.Minute}),
		AnalyticsURL:                         new("https://analytics.example.com"),
		Audiences:                            new([]string{"a1", "a2"}),
		AuthenticateServiceURL:               new("https://authenticate.example.com"),
		BootstrapServiceAccount:              new(true),
		CacheDirectory:                       new("/cache"),
		ChangesetRequiredApprovalCount:       new(uint32(2)),
		CustomerID:                           new("customer-id"),
		DangerouslyDisableAllNamespaceChecks: new(true),
		DatabaseEncryptionKeyFile:            new("/database-encryption-key"),
		DatabaseEncryptionKeyRawFile:         new("/database-encryption-key-raw"),
		DatabrokerServiceURLs:                new([]string{"https://databroker1.example.com", "https://databroker2.example.com"}),
		DebugAddress:                         new(":9000"),
		DebugConfigDump:                      new(true),
		DirectoryProvider:                    new("okta"),
		DirectoryProviderRefreshInterval:     new(metav1.Duration{Duration: 2 * time.Minute}),
		DirectoryProviderRefreshTimeout:      new(metav1.Duration{Duration: 3 * time.Minute}),
		DisableFeedbackWidget:                new(true),
		DisableRemoteDiagnostics:             new(true),
		EnableSessionRecordingQuerierCache:   new(true),
		LicenseKeyValidateOffline:            new(true),
		LogLevel:                             new("debug"),
		MetricsAddress:                       new(":9001"),
		OverrideCertificateName:              new("console.example.com"),
		PrometheusDataDirectory:              new("/prometheus"),
		PrometheusListenAddress:              new(":9002"),
		PrometheusScrapeInterval:             new(metav1.Duration{Duration: 4 * time.Minute}),
		Secret:                               new("enterprise"),
		SharedSecretFile:                     new("/shared-secret"),
		SharedSecretRawFile:                  new("/shared-secret-raw"),
		SigningKeyFile:                       new("/signing-key"),
		SigningKeyRawFile:                    new("/signing-key-raw"),
		TLSCertificateAuthoritySecret:        new("enterprise-ca"),
		TLSDerive:                            new("console.example.com"),
		TLSInsecureSkipVerify:                new(true),
		TLSSecret:                            new("enterprise-tls"),
		URL:                                  new("https://console.example.com"),
		UseStaticAssets:                      new(true),
		ValidationMode:                       new("static"),
	}
	secrets := map[string]*v1.Secret{
		"enterprise": {
			Data: map[string][]byte{
				model.SecretKeyForEnterpriseDatabaseEncryptionKey:    []byte("database-encryption-key"),
				model.SecretKeyForEnterpriseDatabaseURL:              []byte("postgres://database.example.com"),
				model.SecretKeyForEnterpriseDirectoryProviderOptions: []byte(`{"k1":"v1"}`),
				model.SecretKeyForEnterpriseLicenseKey:               []byte("license-key"),
				model.SecretKeyForEnterprisePrometheusURL:            []byte("https://prometheus.example.com"),
				model.SecretKeyForEnterpriseSessionRecordingOptions:  []byte(`{"k2":"v2"}`),
			},
		},
		"enterprise-tls": {
			Data: map[string][]byte{
				v1.TLSCertKey:       []byte("tls-cert"),
				v1.TLSPrivateKeyKey: []byte("tls-key"),
			},
		},
		"enterprise-ca": {
			Data: map[string][]byte{
				model.CAKey: []byte("tls-ca"),
			},
		},
	}
	err := ConvertEnterprise(dst, src, secrets)
	assert.NoError(t, err)
	assert.Equal(t, &config.EnterpriseSettings{
		Administrators:                       nullable.From([]string{"u1", "u2", "u3"}),
		AnalyticsCollectInterval:             nullable.From(time.Minute),
		AnalyticsURL:                         nullable.From("https://analytics.example.com"),
		Audience:                             nullable.From([]string{"a1", "a2"}),
		AuthenticateServiceURL:               nullable.From("https://authenticate.example.com"),
		BindAddress:                          nullable.From(":8701"),
		BootstrapServiceAccount:              nullable.From(true),
		CacheDir:                             nullable.From("/cache"),
		ChangesetRequiredApprovalCount:       nullable.From(uint32(2)),
		CustomerID:                           nullable.From("customer-id"),
		DangerouslyDisableAllNamespaceChecks: nullable.From(true),
		DatabaseEncryptionKey:                nullable.From([]byte("database-encryption-key")),
		DatabaseEncryptionKeyFile:            nullable.From("/database-encryption-key"),
		DatabaseEncryptionKeyRawFile:         nullable.From("/database-encryption-key-raw"),
		DatabaseURL:                          nullable.From("postgres://database.example.com"),
		DatabrokerServiceURLs:                nullable.From([]string{"https://databroker1.example.com", "https://databroker2.example.com"}),
		DebugAddress:                         nullable.From(":9000"),
		DebugConfigDump:                      nullable.From(true),
		DirectoryProvider:                    nullable.From("okta"),
		DirectoryProviderOptions:             nullable.From(map[string]any{"k1": "v1"}),
		DirectoryProviderRefreshInterval:     nullable.From(2 * time.Minute),
		DirectoryProviderRefreshTimeout:      nullable.From(3 * time.Minute),
		DisableFeedbackWidget:                nullable.From(true),
		DisableRemoteDiagnostics:             nullable.From(true),
		EnableSessionRecordingQuerierCache:   nullable.From(true),
		LicenseKey:                           nullable.From("license-key"),
		LicenseKeyValidateOffline:            nullable.From(true),
		LogLevel:                             nullable.From("debug"),
		MetricsAddress:                       nullable.From(":9001"),
		OverrideCertificateName:              nullable.From("console.example.com"),
		PrometheusDataDir:                    nullable.From("/prometheus"),
		PrometheusListenAddress:              nullable.From(":9002"),
		PrometheusScrapeInterval:             nullable.From(4 * time.Minute),
		PrometheusURL:                        nullable.From("https://prometheus.example.com"),
		PublicAddress:                        nullable.From("pomerium-0.pomerium-headless:8701"),
		SessionRecordingOptions:              nullable.From(map[string]any{"k2": "v2"}),
		SharedSecretFile:                     nullable.From("/shared-secret"),
		SharedSecretRawFile:                  nullable.From("/shared-secret-raw"),
		SigningKeyFile:                       nullable.From("/signing-key"),
		SigningKeyRawFile:                    nullable.From("/signing-key-raw"),
		TLSCertificate:                       nullable.From([]byte("tls-cert")),
		TLSCertificateAuthority:              nullable.From([]byte("tls-ca")),
		TLSDerive:                            nullable.From("console.example.com"),
		TLSInsecureSkipVerify:                nullable.From(true),
		TLSKey:                               nullable.From([]byte("tls-key")),
		URL:                                  nullable.From("https://console.example.com"),
		UseStaticAssets:                      nullable.From(true),
		ValidationMode:                       nullable.From(configpb.EnterpriseValidationMode_ENTERPRISE_VALIDATION_MODE_STATIC),
	}, dst)
}

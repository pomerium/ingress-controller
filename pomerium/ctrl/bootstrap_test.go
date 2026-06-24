package ctrl

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/pomerium/config"

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

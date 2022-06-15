package ctrl

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/pomerium/config"

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
	require.NoError(t, applySecrets(&opts, &model.Config{Secrets: secrets}))

	assert.Equal(t, base64.StdEncoding.EncodeToString(secrets.Data["cookie_secret"]), opts.CookieSecret)
	assert.Equal(t, base64.StdEncoding.EncodeToString(secrets.Data["shared_secret"]), opts.SharedKey)
	assert.Equal(t, base64.StdEncoding.EncodeToString(secrets.Data["signing_key"]), opts.SigningKey)
}

func TestSecretsDecodeRules(t *testing.T) {
	var opts config.Options

	assert.NoError(t, applySecrets(&opts, &model.Config{
		Secrets: &v1.Secret{
			Data: map[string][]byte{
				"shared_secret": mustB64Decode(t, "9OkZR6hwfmVD3a7Sfmgq58lUbFJGGz4hl/R9xbHFCAg="),
				"cookie_secret": mustB64Decode(t, "WwMtDXWaRDMBQCylle8OJ+w4kLIDIGd8W3cB4/zFFtg="),
				"signing_key":   mustB64Decode(t, "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUhQbkN5MXk0TEZZVkhQb3RzM05rUSttTXJLcDgvVmVWRkRwaUk2TVNxMlVvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFT1h0VXAxOWFwRnNvVWJoYkI2cExMR1o1WFBXRlE5YWtmeW5ISy9RZ3paNC9MRjZhWEY2egpvS3lHMnNtL2wyajFiQ1JxUGJNd3dEVW9iWFNIODVIeDdRPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),
			},
		},
	}), "ok secret")

	assert.Error(t, applySecrets(&opts, &model.Config{}))
	assert.Error(t, applySecrets(&opts, &model.Config{
		Secrets: &v1.Secret{
			Data: map[string][]byte{
				"cookie_secret": mustB64Decode(t, "WwMtDXWaRDMBQCylle8OJ+w4kLIDIGd8W3cB4/zFFtg="),
				"signing_key":   mustB64Decode(t, "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUhQbkN5MXk0TEZZVkhQb3RzM05rUSttTXJLcDgvVmVWRkRwaUk2TVNxMlVvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFT1h0VXAxOWFwRnNvVWJoYkI2cExMR1o1WFBXRlE5YWtmeW5ISy9RZ3paNC9MRjZhWEY2egpvS3lHMnNtL2wyajFiQ1JxUGJNd3dEVW9iWFNIODVIeDdRPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),
			},
		},
	}))
	assert.Error(t, applySecrets(&opts, &model.Config{
		Secrets: &v1.Secret{
			Data: map[string][]byte{
				"shared_secret": {1, 2, 3},
				"cookie_secret": mustB64Decode(t, "WwMtDXWaRDMBQCylle8OJ+w4kLIDIGd8W3cB4/zFFtg="),
				"signing_key":   mustB64Decode(t, "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUhQbkN5MXk0TEZZVkhQb3RzM05rUSttTXJLcDgvVmVWRkRwaUk2TVNxMlVvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFT1h0VXAxOWFwRnNvVWJoYkI2cExMR1o1WFBXRlE5YWtmeW5ISy9RZ3paNC9MRjZhWEY2egpvS3lHMnNtL2wyajFiQ1JxUGJNd3dEVW9iWFNIODVIeDdRPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),
			},
		},
	}))
}

func NewBootstrapSecrets() {
	panic("unimplemented")
}

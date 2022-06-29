// Package ctrl converts Settings CRD into a bootstrap config
package ctrl

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/url"
	"os"

	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/pomerium/config"

	"github.com/pomerium/ingress-controller/model"
)

// Apply prepares a minimal bootstrap configuration for Pomerium
func Apply(dst *config.Options, src *model.Config) error {
	for _, apply := range []struct {
		name string
		fn   func(*config.Options, *model.Config) error
	}{
		{"secrets", applySecrets},
		{"storage", applyStorage},
	} {
		if err := apply.fn(dst, src); err != nil {
			return fmt.Errorf("%s: %w", apply.name, err)
		}
	}

	if err := dst.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return nil
}

func applyStorage(dst *config.Options, src *model.Config) error {
	if src.Spec.Storage == nil {
		return nil
	}

	if err := src.StorageSecrets.Validate(); err != nil {
		return err
	}

	if src.Spec.Storage.Postgres != nil {
		return applyStoragePostgres(dst, src)
	}
	if src.Spec.Storage.Redis != nil {
		return applyStorageRedis(dst, src)
	}

	return fmt.Errorf("if storage is specified, it must contain either redis or postgresql config. omit storage key for in-memory")
}

func applyStorageRedis(dst *config.Options, src *model.Config) error {
	conn, ok := src.StorageSecrets.Secret.Data[model.StorageConnectionStringKey]
	if !ok {
		return fmt.Errorf("storage secret must have %s key", model.StorageConnectionStringKey)
	}

	dst.DataBrokerStorageConnectionString = string(conn)
	dst.DataBrokerStorageCertSkipVerify = src.Spec.Storage.Redis.TLSSkipVerify

	const prefix = "databroker-redis"
	if src.StorageSecrets.CA != nil {
		ca, err := createFileFromSecret(prefix, "ca", src.StorageSecrets.CA, model.CAKey)
		if err != nil {
			return fmt.Errorf("ca: %w", err)
		}
		dst.DataBrokerStorageCAFile = ca
	}
	if src.StorageSecrets.TLS != nil {
		cert, err := createFileFromSecret(prefix, "cert", src.StorageSecrets.TLS, corev1.TLSCertKey)
		if err != nil {
			return fmt.Errorf("tls cert: %w", err)
		}
		key, err := createFileFromSecret(prefix, "key", src.StorageSecrets.TLS, corev1.TLSPrivateKeyKey)
		if err != nil {
			return fmt.Errorf("tls key: %w", err)
		}
		dst.DataBrokerStorageCertFile = cert
		dst.DataBrokerStorageCertKeyFile = key
	}
	return nil
}

func applyStoragePostgres(dst *config.Options, src *model.Config) error {
	const (
		sslMode     = "sslmode"
		sslRootCert = "sslrootcert"
		sslCert     = "sslcert"
		sslKey      = "sslkey"
		prefix      = "databroker-psql"
	)
	conn, ok := src.StorageSecrets.Secret.Data[model.StorageConnectionStringKey]
	if !ok {
		return fmt.Errorf("storage secret must have %s key", model.StorageConnectionStringKey)
	}

	dst.DataBrokerStorageType = config.StoragePostgresName

	if src.StorageSecrets.CA == nil && src.StorageSecrets.TLS == nil {
		dst.DataBrokerStorageConnectionString = string(conn)
		return nil
	}

	u, err := url.Parse(string(conn))
	if err != nil {
		return fmt.Errorf("parse connection string: %w", err)
	}
	param := u.Query()
	if !param.Has(sslMode) {
		return fmt.Errorf("%s must be set in a connection string if TLS/CA options are provided", sslMode)
	}

	if src.StorageSecrets.CA != nil {
		// in principle, one may customize that externally and provide secrets mounted directly to a pod
		if param.Has(sslRootCert) {
			return fmt.Errorf("%s should not be set in a connection string if CA secret is provided", sslRootCert)
		}
		f, err := createFileFromSecret(prefix, sslRootCert, src.StorageSecrets.CA, model.CAKey)
		if err != nil {
			return fmt.Errorf("ca: %w", err)
		}
		param.Set(sslRootCert, f)
	}

	if src.StorageSecrets.TLS != nil {
		if param.Has(sslCert) || param.Has(sslKey) {
			return fmt.Errorf("%s or %s should not be set in a connection string if TLS secret is provided", sslCert, sslKey)
		}
		cert, err := createFileFromSecret(prefix, sslCert, src.StorageSecrets.TLS, corev1.TLSCertKey)
		if err != nil {
			return fmt.Errorf("tls cert: %w", err)
		}
		key, err := createFileFromSecret(prefix, sslKey, src.StorageSecrets.TLS, corev1.TLSPrivateKeyKey)
		if err != nil {
			return fmt.Errorf("tls key: %w", err)
		}
		param.Set(sslCert, cert)
		param.Set(sslKey, key)
	}

	u.RawQuery = param.Encode()
	dst.DataBrokerStorageConnectionString = u.String()
	return nil
}

func createFileFromSecret(prefix, suffix string, secret *corev1.Secret, key string) (path string, err error) {
	const ownerReadonly = fs.FileMode(0400)

	fd, err := os.CreateTemp("", fmt.Sprintf("%s-%s-*", prefix, suffix))
	if err != nil {
		return "", err
	}
	defer func() {
		errs := multierror.Append(nil, err)
		if err = fd.Close(); err != nil {
			path = ""
			errs = multierror.Append(errs, fmt.Errorf("close: %w", err))
		} else if err = fd.Chmod(ownerReadonly); err != nil {
			path = ""
			err = multierror.Append(errs, fmt.Errorf("chmod readonly: %w", err))
		}
		err = errs.ErrorOrNil()
	}()

	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret %s is missing key %s", types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, key)
	}

	if _, err = fd.Write(data); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}

	return fd.Name(), nil
}

func applySecrets(dst *config.Options, src *model.Config) error {
	if src.Secrets == nil {
		return fmt.Errorf("secrets missing, this is a bug")
	}

	name := types.NamespacedName{Name: src.Secrets.Name, Namespace: src.Secrets.Namespace}

	for _, secret := range []struct {
		key string
		len int
		sp  *string
	}{
		{"shared_secret", 32, &dst.SharedKey},
		{"cookie_secret", 32, &dst.CookieSecret},
		{"signing_key", -1, &dst.SigningKey},
	} {
		data, ok := src.Secrets.Data[secret.key]
		if !ok && secret.len > 0 {
			return fmt.Errorf("secret %s is missing a key %s", name, secret.key)
		}
		if secret.len > 0 && len(data) != secret.len {
			return fmt.Errorf("secret %s, key %s should be %d bytes, got %d", name, secret.key, secret.len, len(data))
		}
		txt := base64.StdEncoding.EncodeToString(data)
		*secret.sp = txt
	}

	return nil
}

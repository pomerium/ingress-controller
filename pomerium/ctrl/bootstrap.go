// Package ctrl converts Settings CRD into a bootstrap config
package ctrl

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/pomerium/config"

	"github.com/pomerium/ingress-controller/internal/filemgr"
	"github.com/pomerium/ingress-controller/model"
)

// Apply prepares a minimal bootstrap configuration for Pomerium
func Apply(ctx context.Context, dst *config.Options, src *model.Config) error {
	for _, apply := range []struct {
		name string
		fn   func(context.Context, *config.Options, *model.Config) error
	}{
		{"secrets", applySecrets},
		{"ca", applyCertificateAuthority},
		{"storage", applyStorage},
	} {
		if err := apply.fn(ctx, dst, src); err != nil {
			return fmt.Errorf("%s: %w", apply.name, err)
		}
	}

	if err := dst.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return nil
}

var storageFiles = filemgr.New(filepath.Join(os.TempDir(), "pomerium-storage-files"))

func applyStorage(ctx context.Context, dst *config.Options, src *model.Config) error {
	if err := storageFiles.DeleteFiles(); err != nil {
		log.FromContext(ctx).V(1).Error(err, "failed to delete existing files")
	}

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

	if src.StorageSecrets.CA != nil {
		ca, err := storageFiles.CreateFile("ca.pem", src.StorageSecrets.Secret.Data[model.CAKey])
		if err != nil {
			return fmt.Errorf("ca: %w", err)
		}
		dst.DataBrokerStorageCAFile = ca
	}
	if src.StorageSecrets.TLS != nil {
		cert, err := storageFiles.CreateFile("cert.pem", src.StorageSecrets.TLS.Data[corev1.TLSCertKey])
		if err != nil {
			return fmt.Errorf("tls cert: %w", err)
		}
		key, err := storageFiles.CreateFile("key.pem", src.StorageSecrets.TLS.Data[corev1.TLSPrivateKeyKey])
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
		f, err := storageFiles.CreateFile(sslRootCert, src.StorageSecrets.CA.Data[model.CAKey])
		if err != nil {
			return fmt.Errorf("ca: %w", err)
		}
		param.Set(sslRootCert, f)
	}

	if src.StorageSecrets.TLS != nil {
		if param.Has(sslCert) || param.Has(sslKey) {
			return fmt.Errorf("%s or %s should not be set in a connection string if TLS secret is provided", sslCert, sslKey)
		}
		cert, err := storageFiles.CreateFile(sslCert, src.StorageSecrets.TLS.Data[corev1.TLSCertKey])
		if err != nil {
			return fmt.Errorf("tls cert: %w", err)
		}
		key, err := storageFiles.CreateFile(sslKey, src.StorageSecrets.TLS.Data[corev1.TLSPrivateKeyKey])
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

func applyCertificateAuthority(ctx context.Context, dst *config.Options, src *model.Config) error {
	if src.CASecret == nil {
		return nil
	}

	dst.CA = base64.StdEncoding.EncodeToString(src.CASecret.Data[model.CAKey])
	return nil
}

func applySecrets(ctx context.Context, dst *config.Options, src *model.Config) error {
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

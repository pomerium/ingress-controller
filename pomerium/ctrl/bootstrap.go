// Package ctrl converts Settings CRD into a bootstrap config
package ctrl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/volatiletech/null/v9"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/pomerium/config"
	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
	"github.com/pomerium/pomerium/pkg/nullable"

	v1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/internal/filemgr"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
)

// Apply prepares a minimal bootstrap configuration for Pomerium
func Apply(ctx context.Context, dst *config.Options, src *model.Config) error {
	for _, apply := range []struct {
		name string
		fn   func(context.Context, *config.Options, *model.Config) error
	}{
		{"authenticate", applyAuthenticate},
		{"databroker", applyDataBroker},
		{"secrets", applySecrets},
		{"storage", applyStorage},
		{"runtime flags", applyRuntimeFlags},
		{"enterprise", applyEnterprise},
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

func applyAuthenticate(_ context.Context, dst *config.Options, src *model.Config) error {
	if src.Spec.IdentityProvider == nil {
		return nil
	}

	// if IdP is set, the authenticate is managed locally,
	// and we need to set the internal URL to the service in order to be able to fetch keys
	// as public URL is not accessible from inside the container due to port remapping
	host, port, err := net.SplitHostPort(dst.Addr)
	if err != nil {
		return fmt.Errorf("parsing server addr: %w", err)
	}
	if host == "" {
		host = "localhost"
	}
	dst.AuthenticateInternalURLString = (&url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(host, port),
	}).String()
	return nil
}

func applyDataBroker(_ context.Context, dst *config.Options, src *model.Config) error {
	if src.Spec.DataBroker == nil {
		return nil
	}

	if src.Spec.DataBroker.ClusterLeaderID == nil {
		return nil
	}

	dst.DataBroker.ClusterLeaderID = null.StringFrom(*src.Spec.DataBroker.ClusterLeaderID)
	return nil
}

func applyRuntimeFlags(_ context.Context, dst *config.Options, src *model.Config) error {
	dst.RuntimeFlags = config.DefaultRuntimeFlags()

	for k, v := range src.Spec.RuntimeFlags {
		dst.RuntimeFlags[config.RuntimeFlag(k)] = v
	}

	return nil
}

func applyStorage(ctx context.Context, dst *config.Options, src *model.Config) error {
	if err := storageFiles.DeleteFiles(); err != nil {
		log.FromContext(ctx).V(1).Error(err, "failed to delete existing files")
	}

	if src.Spec.Storage == nil {
		return nil
	}

	if src.Spec.Storage.File != nil {
		return applyStorageFile(dst, src)
	}

	if err := src.StorageSecrets.Validate(); err != nil {
		return err
	}

	if src.Spec.Storage.Postgres != nil {
		return applyStoragePostgres(dst, src)
	}

	return fmt.Errorf("if storage is specified, it must contain file or postgres config. omit storage key for in-memory")
}

func applyStorageFile(dst *config.Options, src *model.Config) error {
	dst.DataBroker.StorageType = config.StorageFileName
	dst.DataBroker.StorageConnectionString = "file://" + src.Spec.Storage.File.Path
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

	dst.DataBroker.StorageType = config.StoragePostgresName

	if src.StorageSecrets.CA == nil && src.StorageSecrets.TLS == nil {
		dst.DataBroker.StorageConnectionString = string(conn)
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
	dst.DataBroker.StorageConnectionString = u.String()
	return nil
}

func applySecrets(_ context.Context, dst *config.Options, src *model.Config) error {
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

func applyEnterprise(_ context.Context, dst *config.Options, src *model.Config) error {
	if src.Spec.Enterprise == nil {
		return nil
	}

	// only enable enterprise for the first replica
	if dst.DataBroker.ClusterNodeID.String == "pomerium-0" {
		dst.Enterprise.IsSet = true
		return ConvertEnterprise(&dst.Enterprise.Value, src.Spec.Enterprise, src.EnterpriseSecrets)
	}

	return nil
}

// ConvertEnterprise converts the enterprise settings CRD to the protobuf
// enterprise settings.
func ConvertEnterprise(dst *config.EnterpriseSettings, src *v1.Enterprise, secrets map[string]*corev1.Secret) error {
	dst.Administrators = nullable.FromPtr(src.Administrators)
	dst.AnalyticsCollectInterval = convertPtr(src.AnalyticsCollectInterval, func(from metav1.Duration) time.Duration { return from.Duration })
	dst.AnalyticsURL = nullable.FromPtr(src.AnalyticsURL)
	dst.Audience = nullable.FromPtr(src.Audiences)
	dst.AuthenticateServiceURL = nullable.FromPtr(src.AuthenticateServiceURL)
	dst.BindAddress = nullable.From(":8701")
	dst.BootstrapServiceAccount = nullable.FromPtr(src.BootstrapServiceAccount)
	dst.CacheDir = nullable.FromPtr(src.CacheDirectory)
	dst.ChangesetRequiredApprovalCount = nullable.FromPtr(src.ChangesetRequiredApprovalCount)
	dst.CustomerID = nullable.FromPtr(src.CustomerID)
	dst.DangerouslyDisableAllNamespaceChecks = nullable.FromPtr(src.DangerouslyDisableAllNamespaceChecks)
	dst.DatabaseEncryptionKeyFile = nullable.FromPtr(src.DatabaseEncryptionKeyFile)
	dst.DatabaseEncryptionKeyRawFile = nullable.FromPtr(src.DatabaseEncryptionKeyRawFile)
	dst.DatabrokerServiceURLs = nullable.FromPtr(src.DatabrokerServiceURLs)
	dst.DebugAddress = nullable.FromPtr(src.DebugAddress)
	dst.DebugConfigDump = nullable.FromPtr(src.DebugConfigDump)
	dst.DirectoryProvider = nullable.FromPtr(src.DirectoryProvider)
	dst.DirectoryProviderRefreshInterval = convertPtr(src.DirectoryProviderRefreshInterval, func(from metav1.Duration) time.Duration { return from.Duration })
	dst.DirectoryProviderRefreshTimeout = convertPtr(src.DirectoryProviderRefreshTimeout, func(from metav1.Duration) time.Duration { return from.Duration })
	dst.DisableFeedbackWidget = nullable.FromPtr(src.DisableFeedbackWidget)
	dst.DisableRemoteDiagnostics = nullable.FromPtr(src.DisableRemoteDiagnostics)
	dst.EnableSessionRecordingQuerierCache = nullable.FromPtr(src.EnableSessionRecordingQuerierCache)
	dst.LicenseKeyValidateOffline = nullable.FromPtr(src.LicenseKeyValidateOffline)
	dst.LogLevel = nullable.FromPtr(src.LogLevel)
	dst.MetricsAddress = nullable.FromPtr(src.MetricsAddress)
	dst.OverrideCertificateName = nullable.FromPtr(src.OverrideCertificateName)
	dst.PrometheusDataDir = nullable.FromPtr(src.PrometheusDataDirectory)
	dst.PrometheusListenAddress = nullable.FromPtr(src.PrometheusListenAddress)
	dst.PrometheusScrapeInterval = convertPtr(src.PrometheusScrapeInterval, func(from metav1.Duration) time.Duration { return from.Duration })
	dst.PublicAddress = nullable.From("pomerium-0.pomerium-headless:8701")
	dst.SharedSecretFile = nullable.FromPtr(src.SharedSecretFile)
	dst.SharedSecretRawFile = nullable.FromPtr(src.SharedSecretRawFile)
	dst.SigningKeyFile = nullable.FromPtr(src.SigningKeyFile)
	dst.SigningKeyRawFile = nullable.FromPtr(src.SigningKeyRawFile)
	dst.TLSDerive = nullable.FromPtr(src.TLSDerive)
	dst.TLSInsecureSkipVerify = nullable.FromPtr(src.TLSInsecureSkipVerify)
	dst.URL = nullable.FromPtr(src.URL)
	dst.UseStaticAssets = nullable.FromPtr(src.UseStaticAssets)
	dst.ValidationMode = convertPtr(src.ValidationMode, func(from string) configpb.EnterpriseValidationMode {
		switch from {
		case "disabled":
			return configpb.EnterpriseValidationMode_ENTERPRISE_VALIDATION_MODE_DISABLED
		case "static":
			return configpb.EnterpriseValidationMode_ENTERPRISE_VALIDATION_MODE_STATIC
		case "full":
			return configpb.EnterpriseValidationMode_ENTERPRISE_VALIDATION_MODE_FULL
		}
		return configpb.EnterpriseValidationMode_ENTERPRISE_VALIDATION_MODE_UNKNOWN
	})

	for _, mapping := range []struct {
		name string
		src  *string
		key  string
		set  func(value nullable.Value[[]byte]) error
	}{
		{
			name: "Database Encryption Key",
			src:  src.Secret,
			key:  model.SecretKeyForEnterpriseDatabaseEncryptionKey,
			set: func(value nullable.Value[[]byte]) error {
				dst.DatabaseEncryptionKey = value
				return nil
			},
		},
		{
			name: "Database URL",
			src:  src.Secret,
			key:  model.SecretKeyForEnterpriseDatabaseURL,
			set: func(value nullable.Value[[]byte]) error {
				dst.DatabaseURL = convertPtr(value.Ptr(), func(data []byte) string { return string(data) })
				return nil
			},
		},
		{
			name: "Directory Provider Options",
			src:  src.Secret,
			key:  model.SecretKeyForEnterpriseDirectoryProviderOptions,
			set: func(value nullable.Value[[]byte]) error {
				var err error
				dst.DirectoryProviderOptions, err = convertPtrWithError(value.Ptr(), func(data []byte) (map[string]any, error) {
					m := map[string]any{}
					err := json.Unmarshal(data, &m)
					return m, err
				})
				return err
			},
		},
		{
			name: "License Key",
			src:  src.Secret,
			key:  model.SecretKeyForEnterpriseLicenseKey,
			set: func(value nullable.Value[[]byte]) error {
				dst.LicenseKey = convertPtr(value.Ptr(), func(data []byte) string { return string(data) })
				return nil
			},
		},
		{
			name: "Prometheus URL",
			src:  src.Secret,
			key:  model.SecretKeyForEnterprisePrometheusURL,
			set: func(value nullable.Value[[]byte]) error {
				dst.PrometheusURL = convertPtr(value.Ptr(), func(data []byte) string { return string(data) })
				return nil
			},
		},
		{
			name: "Session Recording Options",
			src:  src.Secret,
			key:  model.SecretKeyForEnterpriseSessionRecordingOptions,
			set: func(value nullable.Value[[]byte]) error {
				var err error
				dst.SessionRecordingOptions, err = convertPtrWithError(value.Ptr(), func(data []byte) (map[string]any, error) {
					m := map[string]any{}
					err := json.Unmarshal(data, &m)
					return m, err
				})
				return err
			},
		},
		{
			name: "TLS Certificate",
			src:  src.TLSSecret,
			key:  corev1.TLSCertKey,
			set: func(value nullable.Value[[]byte]) error {
				dst.TLSCertificate = value
				return nil
			},
		},
		{
			name: "TLS Certificate Private Key",
			src:  src.TLSSecret,
			key:  corev1.TLSPrivateKeyKey,
			set: func(value nullable.Value[[]byte]) error {
				dst.TLSKey = value
				return nil
			},
		},
		{
			name: "TLS Certificate Authority",
			src:  src.TLSCertificateAuthoritySecret,
			key:  model.CAKey,
			set: func(value nullable.Value[[]byte]) error {
				dst.TLSCertificateAuthority = value
				return nil
			},
		},
	} {
		if mapping.src == nil {
			if err := mapping.set(nullable.NewValue[[]byte](false, nil)); err != nil {
				return fmt.Errorf("error setting enterprise field %s: %w", mapping.name, err)
			}
			continue
		}

		secret, ok := secrets[*mapping.src]
		if !ok {
			return fmt.Errorf("enterprise secret not found for %s: %s", mapping.name, *mapping.src)
		}
		data, ok := secret.Data[mapping.key]
		if !ok {
			if err := mapping.set(nullable.NewValue[[]byte](false, nil)); err != nil {
				return fmt.Errorf("error setting enterprise field %s: %w", mapping.name, err)
			}
			continue
		}
		if err := mapping.set(nullable.From(data)); err != nil {
			return fmt.Errorf("error setting enterprise field %s: %w", mapping.name, err)
		}
	}

	return nil
}

// ApplyAdditional propagates additional core bootstrap settings, needed when
// running all-in-one mode together with Enterprise API sync.
func ApplyAdditional(ctx context.Context, dst *config.Options, src *model.Config) error {
	var pbConfig configpb.Config
	if err := pomerium.ApplyConfig(ctx, &pbConfig, src); err != nil {
		return err
	}
	dst.ApplySettings(ctx, nil, pbConfig.Settings)
	return nil
}

func convertPtr[From, To any](from *From, fn func(value From) To) nullable.Value[To] {
	if from != nil {
		to := fn(*from)
		return nullable.From(to)
	}
	return nullable.FromPtr[To](nil)
}

func convertPtrWithError[From, To any](from *From, fn func(value From) (To, error)) (nullable.Value[To], error) {
	if from != nil {
		to, err := fn(*from)
		if err != nil {
			return nullable.FromPtr[To](nil), err
		}
		return nullable.From(to), nil
	}
	return nullable.FromPtr[To](nil), nil
}

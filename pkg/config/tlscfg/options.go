// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tlscfg

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
)

// Options describes the configuration properties for TLS Connections.
type Options struct {
	Enabled        bool          `mapstructure:"enabled"`
	CAPath         string        `mapstructure:"ca"`
	CertPath       string        `mapstructure:"cert"`
	KeyPath        string        `mapstructure:"key"`
	ServerName     string        `mapstructure:"server_name"` // only for client-side TLS config
	ClientCAPath   string        `mapstructure:"client_ca"`   // only for server-side TLS config for client auth
	CipherSuites   []string      `mapstructure:"cipher_suites"`
	MinVersion     string        `mapstructure:"min_version"`
	MaxVersion     string        `mapstructure:"max_version"`
	SkipHostVerify bool          `mapstructure:"skip_host_verify"`
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
	certWatcher    *certWatcher
}

var systemCertPool = x509.SystemCertPool // to allow overriding in unit test

// Config loads TLS certificates and returns a TLS Config.
func (o *Options) Config(logger *zap.Logger) (*tls.Config, error) {
	var minVersionId, maxVersionId uint16

	certPool, err := o.loadCertPool()
	if err != nil {
		return nil, fmt.Errorf("failed to load CA CertPool: %w", err)
	}

	cipherSuiteIds, err := CipherSuiteNamesToIDs(o.CipherSuites)
	if err != nil {
		return nil, fmt.Errorf("failed to get cipher suite ids from cipher suite names: %w", err)
	}

	if o.MinVersion != "" {
		minVersionId, err = VersionNameToID(o.MinVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to get minimum tls version: %w", err)
		}
	}

	if o.MaxVersion != "" {
		maxVersionId, err = VersionNameToID(o.MaxVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to get maximum tls version: %w", err)
		}
	}

	if o.MinVersion != "" && o.MaxVersion != "" {
		if minVersionId > maxVersionId {
			return nil, fmt.Errorf("minimum tls version can't be greater than maximum tls version")
		}
	}

	tlsCfg := &tls.Config{
		RootCAs:            certPool,
		ServerName:         o.ServerName,
		InsecureSkipVerify: o.SkipHostVerify, /* #nosec G402*/
		CipherSuites:       cipherSuiteIds,
		MinVersion:         minVersionId,
		MaxVersion:         maxVersionId,
	}

	if o.ClientCAPath != "" {
		// TODO this should be moved to certWatcher, since it already loads key pair
		certPool := x509.NewCertPool()
		if err := addCertToPool(o.ClientCAPath, certPool); err != nil {
			return nil, err
		}
		tlsCfg.ClientCAs = certPool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	certWatcher, err := newCertWatcher(*o, logger, tlsCfg.RootCAs, tlsCfg.ClientCAs)
	if err != nil {
		return nil, err
	}
	o.certWatcher = certWatcher

	if (o.CertPath == "" && o.KeyPath != "") || (o.CertPath != "" && o.KeyPath == "") {
		return nil, fmt.Errorf("for client auth via TLS, either both client certificate and key must be supplied, or neither")
	}
	if o.CertPath != "" && o.KeyPath != "" {
		tlsCfg.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return o.certWatcher.certificate(), nil
		}
		// GetClientCertificate is used on the client side when server is configured with tls.RequireAndVerifyClientCert e.g. mTLS
		tlsCfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return o.certWatcher.certificate(), nil
		}
	}

	return tlsCfg, nil
}

func (o Options) loadCertPool() (*x509.CertPool, error) {
	if len(o.CAPath) == 0 { // no truststore given, use SystemCertPool
		certPool, err := loadSystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load SystemCertPool: %w", err)
		}
		return certPool, nil
	}
	certPool := x509.NewCertPool()
	// setup user specified truststore
	if err := addCertToPool(o.CAPath, certPool); err != nil {
		return nil, err
	}
	return certPool, nil
}

func (o *Options) ToOtelClientConfig() configtls.ClientConfig {
	return configtls.ClientConfig{
		Insecure:           !o.Enabled,
		InsecureSkipVerify: o.SkipHostVerify,
		ServerName:         o.ServerName,
		Config: configtls.Config{
			CAFile:         o.CAPath,
			CertFile:       o.CertPath,
			KeyFile:        o.KeyPath,
			CipherSuites:   o.CipherSuites,
			MinVersion:     o.MinVersion,
			MaxVersion:     o.MaxVersion,
			ReloadInterval: o.ReloadInterval,
		},
	}
}

// ToOtelServerConfig provides a mapping between from Options to OTEL's TLS Server Configuration.
func (o *Options) ToOtelServerConfig() *configtls.ServerConfig {
	if !o.Enabled {
		return nil
	}

	cfg := &configtls.ServerConfig{
		ClientCAFile: o.ClientCAPath,
		Config: configtls.Config{
			CAFile:         o.CAPath,
			CertFile:       o.CertPath,
			KeyFile:        o.KeyPath,
			CipherSuites:   o.CipherSuites,
			MinVersion:     o.MinVersion,
			MaxVersion:     o.MaxVersion,
			ReloadInterval: o.ReloadInterval,
		},
	}

	if o.ReloadInterval > 0 {
		cfg.ReloadClientCAFile = true
	}

	return cfg
}

func addCertToPool(caPath string, certPool *x509.CertPool) error {
	caPEM, err := os.ReadFile(filepath.Clean(caPath))
	if err != nil {
		return fmt.Errorf("failed to load CA %s: %w", caPath, err)
	}

	if !certPool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("failed to parse CA %s", caPath)
	}
	return nil
}

var _ io.Closer = (*Options)(nil)

// Close shuts down the embedded certificate watcher.
func (o *Options) Close() error {
	if o.certWatcher != nil {
		return o.certWatcher.Close()
	}
	return nil
}

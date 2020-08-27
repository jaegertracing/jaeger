// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tlscfg

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"go.uber.org/zap"
)

// Options describes the configuration properties for TLS Connections.
type Options struct {
	Enabled        bool         `mapstructure:"enabled"`
	CAPath         string       `mapstructure:"ca"`
	CertPath       string       `mapstructure:"cert"`
	KeyPath        string       `mapstructure:"key"`
	ServerName     string       `mapstructure:"server_name"` // only for client-side TLS config
	ClientCAPath   string       `mapstructure:"client_ca"`   // only for server-side TLS config for client auth
	SkipHostVerify bool         `mapstructure:"skip_host_verify"`
	certWatcher    *certWatcher `mapstructure:"-"`
}

var systemCertPool = x509.SystemCertPool // to allow overriding in unit test

// Config loads TLS certificates and returns a TLS Config.
func (p *Options) Config(logger *zap.Logger) (*tls.Config, error) {

	certPool, err := p.loadCertPool()
	if err != nil {
		return nil, fmt.Errorf("failed to load CA CertPool: %w", err)
	}
	// #nosec G402
	tlsCfg := &tls.Config{
		RootCAs:            certPool,
		ServerName:         p.ServerName,
		InsecureSkipVerify: p.SkipHostVerify,
	}
	if p.ClientCAPath != "" {
		certPool := x509.NewCertPool()
		if err := addCertToPool(p.ClientCAPath, certPool); err != nil {
			return nil, err
		}
		tlsCfg.ClientCAs = certPool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	w, err := newCertWatcher(*p, logger)
	if err != nil {
		return nil, err
	}
	p.certWatcher = w

	if (p.CertPath == "" && p.KeyPath != "") || (p.CertPath != "" && p.KeyPath == "") {
		return nil, fmt.Errorf("for client auth via TLS, either both client certificate and key must be supplied, or neither")
	}
	if p.CertPath != "" && p.KeyPath != "" {
		tlsCfg.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return p.certWatcher.certificate(), nil
		}
		// GetClientCertificate is used on the client side when server is configured with tls.RequireAndVerifyClientCert e.g. mTLS
		tlsCfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return p.certWatcher.certificate(), nil
		}
	}

	go p.certWatcher.watchChangesLoop(tlsCfg.RootCAs, tlsCfg.ClientCAs)
	return tlsCfg, nil
}

func (p Options) loadCertPool() (*x509.CertPool, error) {
	if len(p.CAPath) == 0 { // no truststore given, use SystemCertPool
		certPool, err := systemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load SystemCertPool: %w", err)
		}
		return certPool, nil
	}
	certPool := x509.NewCertPool()
	// setup user specified truststore
	if err := addCertToPool(p.CAPath, certPool); err != nil {
		return nil, err
	}
	return certPool, nil
}

func addCertToPool(caPath string, certPool *x509.CertPool) error {
	caPEM, err := ioutil.ReadFile(filepath.Clean(caPath))
	if err != nil {
		return fmt.Errorf("failed to load CA %s: %w", caPath, err)
	}

	if !certPool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("failed to parse CA %s", caPath)
	}
	return nil
}

var _ io.Closer = (*Options)(nil)

// Close closes Options.
func (p *Options) Close() error {
	if p.certWatcher != nil {
		return p.certWatcher.Close()
	}
	return nil
}

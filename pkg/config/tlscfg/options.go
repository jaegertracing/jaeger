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
	"io/ioutil"
	"path/filepath"
)

// Options describes the configuration properties for TLS Connections.
type Options struct {
	Enabled        bool   `mapstructure:"enabled"`
	CAPath         string `mapstructure:"ca"`
	CertPath       string `mapstructure:"cert"`
	KeyPath        string `mapstructure:"key"`
	ServerName     string `mapstructure:"server_name"` // only for client-side TLS config
	ClientCAPath   string `mapstructure:"client_ca"`   // only for server-side TLS config for client auth
	SkipHostVerify bool   `mapstructure:"skip_host_verify"`
}

var systemCertPool = x509.SystemCertPool // to allow overriding in unit test

// Config loads TLS certificates and returns a TLS Config.
func (p Options) Config() (*tls.Config, error) {
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

	if (p.CertPath == "" && p.KeyPath != "") || (p.CertPath != "" && p.KeyPath == "") {
		return nil, fmt.Errorf("for client auth via TLS, either both client certificate and key must be supplied, or neither")
	}
	if p.CertPath != "" && p.KeyPath != "" {
		tlsCert, err := tls.LoadX509KeyPair(filepath.Clean(p.CertPath), filepath.Clean(p.KeyPath))
		if err != nil {
			return nil, fmt.Errorf("failed to load server TLS cert and key: %w", err)
		}
		tlsCfg.Certificates = append(tlsCfg.Certificates, tlsCert)
	}

	if p.ClientCAPath != "" {
		certPool, err := p.loadCert(p.ClientCAPath)
		if err != nil {
			return nil, err
		}
		tlsCfg.ClientCAs = certPool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

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
	// setup user specified truststore
	return p.loadCert(p.CAPath)
}

func (p Options) loadCert(caPath string) (*x509.CertPool, error) {
	caPEM, err := ioutil.ReadFile(filepath.Clean(caPath))
	if err != nil {
		return nil, fmt.Errorf("failed to load CA %s: %w", caPath, err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse CA %s", caPath)
	}
	return certPool, nil
}

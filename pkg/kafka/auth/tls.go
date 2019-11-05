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

package auth

import (
	gotls "crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"path/filepath"

	"github.com/Shopify/sarama"
	"github.com/pkg/errors"
)

// TLSConfig describes the configuration properties for TLS Connections to the Kafka Brokers
type TLSConfig struct {
	CertPath string
	KeyPath  string
	CaPath   string
}

func setTLSConfiguration(config *TLSConfig, saramaConfig *sarama.Config) error {
	tlsConfig, err := config.getTLS()
	if err != nil {
		return errors.Wrap(err, "error loading tls config")
	}
	saramaConfig.Net.TLS.Enable = true
	saramaConfig.Net.TLS.Config = tlsConfig
	return nil
}

func (tlsConfig TLSConfig) getTLS() (*gotls.Config, error) {
	ca, err := loadCA(tlsConfig.CaPath)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading ca")
	}

	cert, err := gotls.LoadX509KeyPair(filepath.Clean(tlsConfig.CertPath), filepath.Clean(tlsConfig.KeyPath))
	if err != nil {
		return nil, errors.Wrap(err, "error loading certificate")
	}

	return &gotls.Config{
		RootCAs:      ca,
		Certificates: []gotls.Certificate{cert},
	}, nil
}

func loadCA(caPath string) (*x509.CertPool, error) {
	caBytes, err := ioutil.ReadFile(filepath.Clean(caPath))
	if err != nil {
		return nil, errors.Wrapf(err, "error reading caFile %s", caPath)
	}
	certificates := x509.NewCertPool()
	if ok := certificates.AppendCertsFromPEM(caBytes); !ok {
		return nil, errors.Errorf("no ca certificates could be parsed")
	}
	return certificates, nil
}

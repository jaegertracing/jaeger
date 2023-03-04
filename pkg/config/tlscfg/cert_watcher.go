// Copyright (c) 2020 The Jaeger Authors.
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
	"path/filepath"
	"sync"

	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/fswatcher"
)

// certWatcher watches filesystem changes on certificates supplied via Options
// The changed RootCAs and ClientCAs certificates are added to x509.CertPool without invalidating the previously used certificate.
// The certificate and key can be obtained via certWatcher.certificate.
// The consumers of this API should use GetCertificate or GetClientCertificate from tls.Config to supply the certificate to the config.
type certWatcher struct {
	mu       sync.RWMutex
	opts     Options
	logger   *zap.Logger
	watchers []fswatcher.FSWatcher
	cert     *tls.Certificate
}

var _ io.Closer = (*certWatcher)(nil)

func newCertWatcher(opts Options, logger *zap.Logger, tlsCfg *tls.Config) (*certWatcher, error) {
	var cert *tls.Certificate
	if opts.CertPath != "" && opts.KeyPath != "" {
		// load certs at startup to catch missing certs error early
		c, err := tls.LoadX509KeyPair(filepath.Clean(opts.CertPath), filepath.Clean(opts.KeyPath))
		if err != nil {
			return nil, fmt.Errorf("failed to load server TLS cert and key: %w", err)
		}
		cert = &c
	}

	w := &certWatcher{
		opts:   opts,
		logger: logger,
		cert:   cert,
	}

	w.watchCertPair()
	w.watchCert(w.opts.CAPath, tlsCfg.RootCAs)
	w.watchCert(w.opts.ClientCAPath, tlsCfg.ClientCAs)

	return w, nil
}

func (w *certWatcher) Close() error {
	var err error
	for _, w := range w.watchers {
		err = multierr.Append(err, w.Close())
	}
	return err
}

func (w *certWatcher) certificate() *tls.Certificate {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cert
}

func (w *certWatcher) watchCertPair() {
	onCertPairChange := func() {
		c, err := tls.LoadX509KeyPair(filepath.Clean(w.opts.CertPath), filepath.Clean(w.opts.KeyPath))
		if err == nil {
			w.mu.Lock()
			w.cert = &c
			w.mu.Unlock()
			w.logger.Info("Loaded modified certificate", zap.String("certificate", w.opts.CertPath))
			w.logger.Info("Loaded modified certificate", zap.String("certificate", w.opts.KeyPath))
		} else {
			w.logger.Error(
				"Failed to load certificate pair",
				zap.String("certificate", w.opts.CertPath),
				zap.String("key", w.opts.KeyPath),
				zap.Error(err),
			)
		}
	}

	certPairWatcher, err := fswatcher.NewFSWatcher([]string{w.opts.CertPath, w.opts.KeyPath}, onCertPairChange, w.logger)
	if err == nil {
		w.watchers = append(w.watchers, *certPairWatcher)
	} else {
		w.logger.Error(
			"Cannot set up watcher for certificate",
			zap.String("certificate", w.opts.CertPath),
			zap.String("key", w.opts.KeyPath),
			zap.Error(err),
		)
		w.Close()
	}
}

func (w *certWatcher) watchCert(certPath string, certPool *x509.CertPool) {
	onCertChange := func() {
		if err := addCertToPool(certPath, certPool); err == nil {
			w.logger.Info("Loaded modified certificate", zap.String("certificate", certPath))
		} else {
			w.logger.Error("Failed to load certificate", zap.String("certificate", certPath), zap.Error(err))
		}
	}

	certWatcher, err := fswatcher.NewFSWatcher([]string{certPath}, onCertChange, w.logger)
	if err == nil {
		w.watchers = append(w.watchers, *certWatcher)
	} else {
		w.logger.Error("Cannot set up watcher for certificate", zap.String("certificate", certPath), zap.Error(err))
		w.Close()
	}
}

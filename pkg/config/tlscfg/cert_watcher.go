// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tlscfg

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/fswatcher"
)

const (
	logMsgPairReloaded    = "Reloaded modified key pair"
	logMsgCertReloaded    = "Reloaded modified certificate"
	logMsgPairNotReloaded = "Failed to reload key pair, using previous versions"
	logMsgCertNotReloaded = "Failed to reload certificate, using previous version"
)

// certWatcher watches filesystem changes on certificates supplied via Options
// The changed RootCAs and ClientCAs certificates are added to x509.CertPool without invalidating the previously used certificate.
// The certificate and key can be obtained via certWatcher.certificate.
// The consumers of this API should use GetCertificate or GetClientCertificate from tls.Config to supply the certificate to the config.
type certWatcher struct {
	mu       sync.RWMutex
	opts     Options
	logger   *zap.Logger
	watchers []*fswatcher.FSWatcher
	cert     *tls.Certificate
}

var _ io.Closer = (*certWatcher)(nil)

func newCertWatcher(opts Options, logger *zap.Logger, rootCAs, clientCAs *x509.CertPool) (*certWatcher, error) {
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

	if err := w.watchCertPair(); err != nil {
		return nil, err
	}
	if err := w.watchCert(w.opts.CAPath, rootCAs); err != nil {
		return nil, err
	}
	if err := w.watchCert(w.opts.ClientCAPath, clientCAs); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *certWatcher) Close() error {
	var errs []error
	for _, w := range w.watchers {
		errs = append(errs, w.Close())
	}
	return errors.Join(errs...)
}

func (w *certWatcher) certificate() *tls.Certificate {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cert
}

func (w *certWatcher) watchCertPair() error {
	watcher, err := fswatcher.New(
		[]string{w.opts.CertPath, w.opts.KeyPath},
		w.onCertPairChange,
		w.logger,
	)
	if err == nil {
		w.watchers = append(w.watchers, watcher)
		return nil
	}
	w.Close()
	return fmt.Errorf("failed to watch key pair %s and %s: %w", w.opts.KeyPath, w.opts.CertPath, err)
}

func (w *certWatcher) watchCert(certPath string, certPool *x509.CertPool) error {
	onCertChange := func() { w.onCertChange(certPath, certPool) }

	watcher, err := fswatcher.New([]string{certPath}, onCertChange, w.logger)
	if err == nil {
		w.watchers = append(w.watchers, watcher)
		return nil
	}
	w.Close()
	return fmt.Errorf("failed to watch cert %s: %w", certPath, err)
}

func (w *certWatcher) onCertPairChange() {
	cert, err := tls.LoadX509KeyPair(filepath.Clean(w.opts.CertPath), filepath.Clean(w.opts.KeyPath))
	if err == nil {
		w.mu.Lock()
		w.cert = &cert
		w.mu.Unlock()
		w.logger.Info(
			logMsgPairReloaded,
			zap.String("key", w.opts.KeyPath),
			zap.String("cert", w.opts.CertPath),
		)
	} else {
		w.logger.Error(
			logMsgPairNotReloaded,
			zap.String("key", w.opts.KeyPath),
			zap.String("cert", w.opts.CertPath),
			zap.Error(err),
		)
	}
}

func (w *certWatcher) onCertChange(certPath string, certPool *x509.CertPool) {
	w.mu.Lock() // prevent concurrent updates to the same certPool
	if err := addCertToPool(certPath, certPool); err == nil {
		w.logger.Info(logMsgCertReloaded, zap.String("cert", certPath))
	} else {
		w.logger.Error(logMsgCertNotReloaded, zap.String("cert", certPath), zap.Error(err))
	}
	w.mu.Unlock()
}

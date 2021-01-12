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

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// certWatcher watches filesystem changes on certificates supplied via Options
// The changed RootCAs and ClientCAs certificates are added to x509.CertPool without invalidating the previously used certificate.
// The certificate and key can be obtained via certWatcher.certificate.
// The consumers of this API should use GetCertificate or GetClientCertificate from tls.Config to supply the certificate to the config.
type certWatcher struct {
	opts    Options
	watcher *fsnotify.Watcher
	cert    *tls.Certificate
	logger  *zap.Logger
	mu      *sync.RWMutex
}

var _ io.Closer = (*certWatcher)(nil)

func newCertWatcher(opts Options, logger *zap.Logger) (*certWatcher, error) {
	var cert *tls.Certificate
	if opts.CertPath != "" && opts.KeyPath != "" {
		// load certs at startup to catch missing certs error early
		c, err := tls.LoadX509KeyPair(filepath.Clean(opts.CertPath), filepath.Clean(opts.KeyPath))
		if err != nil {
			return nil, fmt.Errorf("failed to load server TLS cert and key: %w", err)
		}
		cert = &c
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := addCertsToWatch(watcher, opts); err != nil {
		watcher.Close()
		return nil, err
	}
	return &certWatcher{
		cert:    cert,
		opts:    opts,
		watcher: watcher,
		logger:  logger,
		mu:      &sync.RWMutex{},
	}, nil
}

func (w *certWatcher) Close() error {
	return w.watcher.Close()
}

func (w *certWatcher) certificate() *tls.Certificate {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cert
}

func (w *certWatcher) watchChangesLoop(rootCAs, clientCAs *x509.CertPool) {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// ignore if the event is a chmod event (permission or owner changes)
			if event.Op&fsnotify.Chmod == fsnotify.Chmod {
				continue
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				w.logger.Warn("Certificate has been removed, using the last known version",
					zap.String("certificate", event.Name))
				continue
			}

			w.logger.Info("Loading modified certificate",
				zap.String("certificate", event.Name),
				zap.String("event", event.Op.String()))
			var err error
			switch event.Name {
			case w.opts.CAPath:
				err = addCertToPool(w.opts.CAPath, rootCAs)
			case w.opts.ClientCAPath:
				err = addCertToPool(w.opts.ClientCAPath, clientCAs)
			case w.opts.CertPath, w.opts.KeyPath:
				w.mu.Lock()
				c, e := tls.LoadX509KeyPair(filepath.Clean(w.opts.CertPath), filepath.Clean(w.opts.KeyPath))
				if e == nil {
					w.cert = &c
				}
				w.mu.Unlock()
				err = e
			}
			if err == nil {
				w.logger.Info("Loaded modified certificate",
					zap.String("certificate", event.Name),
					zap.String("event", event.Op.String()))

			} else {
				w.logger.Error("Failed to load certificate",
					zap.String("certificate", event.Name),
					zap.String("event", event.Op.String()),
					zap.Error(err))
			}
		case err := <-w.watcher.Errors:
			w.logger.Error("Watcher got error", zap.Error(err))
		}
	}
}

func addCertsToWatch(watcher *fsnotify.Watcher, opts Options) error {
	if len(opts.CAPath) != 0 {
		err := watcher.Add(opts.CAPath)
		if err != nil {
			return err
		}
	}
	if len(opts.ClientCAPath) != 0 {
		err := watcher.Add(opts.ClientCAPath)
		if err != nil {
			return err
		}
	}
	if len(opts.CertPath) != 0 {
		err := watcher.Add(opts.CertPath)
		if err != nil {
			return err
		}
	}
	if len(opts.KeyPath) != 0 {
		err := watcher.Add(opts.KeyPath)
		if err != nil {
			return err
		}
	}
	return nil
}

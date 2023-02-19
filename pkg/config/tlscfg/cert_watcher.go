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
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/fswatcher"
)

// certWatcher watches filesystem changes on certificates supplied via Options
// The changed RootCAs and ClientCAs certificates are added to x509.CertPool without invalidating the previously used certificate.
// The certificate and key can be obtained via certWatcher.certificate.
// The consumers of this API should use GetCertificate or GetClientCertificate from tls.Config to supply the certificate to the config.
type certWatcher struct {
	mu           sync.RWMutex
	opts         Options
	logger       *zap.Logger
	watcher      fswatcher.Watcher
	cert         *tls.Certificate
	caHash       string
	clientCAHash string
	certHash     string
	keyHash      string
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

	fsw, err := fswatcher.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &certWatcher{
		opts:    opts,
		logger:  logger,
		cert:    cert,
		watcher: fsw,
	}

	return w, nil
}

func (w *certWatcher) Close() error {
	return w.watcher.Close()
}

func (w *certWatcher) certificate() *tls.Certificate {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cert
}

// setupWatchedPaths retrieves hashes of all configured certificates
// and adds their parent directories to watched paths.
func (w *certWatcher) setupWatchedPaths() ([]string, error) {
	watchedPaths := make([]string, 1)
	uniqueDirs := make(map[string]bool)
	addPath := func(certPath string, hashPtr *string) error {
		if certPath == "" {
			return nil
		}
		if h, err := hashFile(certPath); err == nil {
			*hashPtr = h
		} else {
			return err
		}
		dir := path.Dir(certPath)
		if _, ok := uniqueDirs[dir]; !ok {
			watchedPaths = append(watchedPaths, dir)
			uniqueDirs[dir] = true
		}
		return nil
	}

	if err := addPath(w.opts.CAPath, &w.caHash); err != nil {
		return nil, err
	}
	if err := addPath(w.opts.ClientCAPath, &w.clientCAHash); err != nil {
		return nil, err
	}
	if err := addPath(w.opts.CertPath, &w.certHash); err != nil {
		return nil, err
	}
	if err := addPath(w.opts.KeyPath, &w.keyHash); err != nil {
		return nil, err
	}
	return watchedPaths, nil
}

// attemptReload checks if the watched files have been modified and reloads them if necessary.
func (w *certWatcher) attemptReload(rootCAs, clientCAs *x509.CertPool) {
	w.reloadIfModified(w.opts.CAPath, &w.caHash, rootCAs)
	w.reloadIfModified(w.opts.ClientCAPath, &w.clientCAHash, clientCAs)

	isCertModified, newCertHash := w.isModified(w.opts.CertPath, w.certHash)
	isKeyModified, newKeyHash := w.isModified(w.opts.KeyPath, w.keyHash)
	if isCertModified || isKeyModified {
		c, err := tls.LoadX509KeyPair(filepath.Clean(w.opts.CertPath), filepath.Clean(w.opts.KeyPath))
		if err == nil {
			w.mu.Lock()
			w.cert = &c
			w.certHash = newCertHash
			w.keyHash = newKeyHash
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
}

func (w *certWatcher) reloadIfModified(certPath string, certHash *string, certPool *x509.CertPool) {
	if mod, newHash := w.isModified(certPath, *certHash); mod {
		if err := addCertToPool(certPath, certPool); err == nil {
			w.mu.Lock()
			*certHash = newHash
			w.mu.Unlock()
			w.logger.Info("Loaded modified certificate", zap.String("certificate", certPath))
		} else {
			w.logger.Error("Failed to load certificate", zap.String("certificate", certPath), zap.Error(err))
		}
	}
}

// isModified returns true if the file has been modified since the last check.
func (w *certWatcher) isModified(file string, previousHash string) (bool, string) {
	if file == "" {
		return false, ""
	}
	hash, err := hashFile(file)
	if err != nil {
		w.logger.Warn("Certificate has been removed, using the last known version", zap.String("certificate", file))
		return false, ""
	}
	return previousHash != hash, hash
}

// hashFile returns the SHA256 hash of the file.
func hashFile(file string) (string, error) {
	f, err := os.Open(filepath.Clean(file))
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

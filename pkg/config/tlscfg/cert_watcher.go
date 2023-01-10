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

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// certWatcher watches filesystem changes on certificates supplied via Options
// The changed RootCAs and ClientCAs certificates are added to x509.CertPool without invalidating the previously used certificate.
// The certificate and key can be obtained via certWatcher.certificate.
// The consumers of this API should use GetCertificate or GetClientCertificate from tls.Config to supply the certificate to the config.
type certWatcher struct {
	opts         Options
	watcher      *fsnotify.Watcher
	cert         *tls.Certificate
	logger       *zap.Logger
	mu           *sync.RWMutex
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
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &certWatcher{
		cert:    cert,
		opts:    opts,
		watcher: watcher,
		logger:  logger,
		mu:      &sync.RWMutex{},
	}
	if err := w.addWatches(watcher, opts); err != nil {
		watcher.Close()
		return nil, err
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

func (w *certWatcher) watchChangesLoop(rootCAs, clientCAs *x509.CertPool) {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			w.logger.Debug("Received event", zap.String("event", event.String()))
			if !ok {
				return
			}

			// Write and Rename events indicate that some files might have changed and reload might be necessary.
			// Remove event indicates that the file was deleted and we should write an error to log.
			//
			// Reasoning:
			//
			// Write event is sent if the file content is rewritten.
			//
			// Usually files are not rewritten, but they are updated by swapping them with new
			// ones by calling Rename. That avoids files being read while they are not yet
			// completely written but it also means that inotify on file level will not work:
			// watch is invalidated when the old file is deleted.
			//
			// If reading from Kubernetes Secret volumes the target files are symbolic links
			// to files in a different directory. That directory is swapped with a new one,
			// while the symbolic links remain the same. This guarantees atomic swap for all
			// files at once, but it also means any Rename event in the directory might
			// indicate that the files were replaced, even if event.Name is not any of the
			// files we are monitoring. We check the hashes of the files to detect if they
			// were really changed.
			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Rename == fsnotify.Rename ||
				event.Op&fsnotify.Remove == fsnotify.Remove {
				w.attemptReload(rootCAs, clientCAs)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("Watcher got error", zap.Error(err))
		}
	}
}

func (w *certWatcher) addWatches(watcher *fsnotify.Watcher, opts Options) error {
	// Get initial hashes of the files so that we can detect changes.
	// Build a list of parent directories.
	var dirs []string
	var err error
	if opts.CAPath != "" {
		w.caHash, err = hashFile(opts.CAPath)
		if err != nil {
			return err
		}
		dirs = append(dirs, path.Dir(opts.CAPath))
	}
	if opts.ClientCAPath != "" {
		w.clientCAHash, err = hashFile(opts.ClientCAPath)
		if err != nil {
			return err
		}
		dirs = append(dirs, path.Dir(opts.ClientCAPath))
	}
	if opts.CertPath != "" {
		w.certHash, err = hashFile(opts.CertPath)
		if err != nil {
			return err
		}
		dirs = append(dirs, path.Dir(opts.CertPath))
	}
	if opts.KeyPath != "" {
		w.keyHash, err = hashFile(opts.KeyPath)
		if err != nil {
			return err
		}
		dirs = append(dirs, path.Dir(opts.KeyPath))
	}

	// Find unique directories and add watches.
	uniqueDirs := make(map[string]bool)
	for _, p := range dirs {
		if _, ok := uniqueDirs[p]; !ok {
			err := watcher.Add(p)
			if err != nil {
				return err
			}
		}
		uniqueDirs[p] = true
	}
	return nil
}

// attemptReload checks if the watched files have been modified and reloads them if necessary.
func (w *certWatcher) attemptReload(rootCAs, clientCAs *x509.CertPool) {
	if isModified, newHash := w.isModified(w.opts.CAPath, w.caHash); isModified {
		err := addCertToPool(w.opts.CAPath, rootCAs)
		if err != nil {
			w.logger.Error("Failed to load certificate", zap.String("certificate", w.opts.CAPath), zap.Error(err))
		} else {
			w.caHash = newHash
			w.logger.Info("Loaded modified certificate", zap.String("certificate", w.opts.CAPath))
		}
	}

	if isModified, newHash := w.isModified(w.opts.ClientCAPath, w.clientCAHash); isModified {
		err := addCertToPool(w.opts.ClientCAPath, clientCAs)
		if err != nil {
			w.logger.Error("Failed to load certificate", zap.String("certificate", w.opts.ClientCAPath), zap.Error(err))
		} else {
			w.clientCAHash = newHash
			w.logger.Info("Loaded modified certificate", zap.String("certificate", w.opts.ClientCAPath))
		}
	}

	isCertModified, newCertHash := w.isModified(w.opts.CertPath, w.certHash)
	isKeyModified, newKeyHash := w.isModified(w.opts.KeyPath, w.keyHash)
	if isCertModified || isKeyModified {
		c, err := tls.LoadX509KeyPair(filepath.Clean(w.opts.CertPath), filepath.Clean(w.opts.KeyPath))
		if err != nil {
			w.logger.Error("Failed to load certificate",
				zap.String("certificate", w.opts.CertPath), zap.String("key", w.opts.KeyPath), zap.Error(err))
		} else {
			w.mu.Lock()
			w.cert = &c
			w.certHash = newCertHash
			w.keyHash = newKeyHash
			w.mu.Unlock()
			w.logger.Info("Loaded modified certificate", zap.String("certificate", w.opts.CertPath))
			w.logger.Info("Loaded modified certificate", zap.String("certificate", w.opts.KeyPath))
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

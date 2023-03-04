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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	serverCert = "./testdata/example-server-cert.pem"
	serverKey  = "./testdata/example-server-key.pem"
	clientCert = "./testdata/example-client-cert.pem"
	clientKey  = "./testdata/example-client-key.pem"

	caCert      = "./testdata/example-CA-cert.pem"
	wrongCaCert = "./testdata/wrong-CA-cert.pem"
	badCaCert   = "./testdata/bad-CA-cert.txt"
)

func copyToTempFile(t *testing.T, pattern string, filename string) (file *os.File, closeFn func()) {
	tempFile, err := os.CreateTemp("", pattern)
	require.NoError(t, err)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	_, err = tempFile.Write(data)
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	return tempFile, func() {
		// ignore error because some tests may remove the files earlier
		_ = os.Remove(tempFile.Name())
	}
}

func copyFile(t *testing.T, dest string, src string) {
	certData, err := os.ReadFile(src)
	require.NoError(t, err)
	err = syncWrite(dest, certData, 0o644)
	require.NoError(t, err)
}

func TestReload(t *testing.T) {
	// copy certs to temp so we can modify them
	certFile, certFileCloseFn := copyToTempFile(t, "cert.crt", serverCert)
	defer certFileCloseFn()

	keyFile, keyFileCloseFn := copyToTempFile(t, "key.crt", serverKey)
	defer keyFileCloseFn()

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	opts := Options{
		CAPath:       caCert,
		ClientCAPath: caCert,
		CertPath:     certFile.Name(),
		KeyPath:      keyFile.Name(),
	}
	watcher, err := newCertWatcher(opts, logger)
	require.NoError(t, err)
	assert.NotNil(t, watcher.certificate())
	defer watcher.Close()

	certPool := x509.NewCertPool()
	require.NoError(t, err)
	watcher.watchCertPair()
	watcher.watchCert(watcher.opts.CAPath, certPool)
	watcher.watchCert(watcher.opts.ClientCAPath, certPool)
	cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
	require.NoError(t, err)
	assert.Equal(t, &cert, watcher.certificate())

	// Write the client's public key.
	copyFile(t, certFile.Name(), clientCert)

	assertLogs(t,
		func() bool {
			// Logged when the cert is reloaded with mismatching client public key and existing server private key.
			return logObserver.FilterMessage("Failed to load certificate pair").
				FilterField(zap.String("certificate", certFile.Name())).Len() > 0
		},
		"Unable to locate 'Failed to load certificate pair' in log. All logs: %v", logObserver)

	// Write the client's private key.
	copyFile(t, keyFile.Name(), clientKey)

	assertLogs(t,
		func() bool {
			// Logged when the client private key is modified in the cert which enables successful reloading of
			// the cert as both private and public keys now match.
			return logObserver.FilterMessage("Loaded modified certificate").
				FilterField(zap.String("certificate", keyFile.Name())).Len() > 0
		},
		"Unable to locate 'Loaded modified certificate' in log. All logs: %v", logObserver)

	cert, err = tls.LoadX509KeyPair(filepath.Clean(clientCert), clientKey)
	require.NoError(t, err)
	assert.Equal(t, &cert, watcher.certificate())
}

func TestReload_ca_certs(t *testing.T) {
	// copy certs to temp so we can modify them
	caFile, caFileCloseFn := copyToTempFile(t, "cert.crt", caCert)
	defer caFileCloseFn()
	clientCaFile, clientCaFileClostFn := copyToTempFile(t, "key.crt", caCert)
	defer clientCaFileClostFn()

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	opts := Options{
		CAPath:       caFile.Name(),
		ClientCAPath: clientCaFile.Name(),
	}
	watcher, err := newCertWatcher(opts, logger)
	require.NoError(t, err)
	defer watcher.Close()

	certPool := x509.NewCertPool()
	require.NoError(t, err)
	watcher.watchCert(watcher.opts.CAPath, certPool)
	watcher.watchCert(watcher.opts.ClientCAPath, certPool)

	// update the content with different certs to trigger reload.
	copyFile(t, caFile.Name(), wrongCaCert)
	copyFile(t, clientCaFile.Name(), wrongCaCert)

	assertLogs(t,
		func() bool {
			return logObserver.FilterField(zap.String("certificate", caFile.Name())).Len() > 0
		},
		"Unable to locate 'certificate' in log. All logs: %v", logObserver)

	assertLogs(t,
		func() bool {
			return logObserver.FilterField(zap.String("certificate", clientCaFile.Name())).Len() > 0
		},
		"Unable to locate 'certificate' in log. All logs: %v", logObserver)
}

func TestReload_err_cert_update(t *testing.T) {
	// copy certs to temp so we can modify them
	certFile, certFileCloseFn := copyToTempFile(t, "cert.crt", serverCert)
	defer certFileCloseFn()
	keyFile, keyFileCloseFn := copyToTempFile(t, "cert.crt", serverKey)
	defer keyFileCloseFn()

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	opts := Options{
		CAPath:       caCert,
		ClientCAPath: caCert,
		CertPath:     certFile.Name(),
		KeyPath:      keyFile.Name(),
	}
	watcher, err := newCertWatcher(opts, logger)
	require.NoError(t, err)
	assert.NotNil(t, watcher.certificate())
	defer watcher.Close()

	certPool := x509.NewCertPool()
	require.NoError(t, err)
	watcher.watchCertPair()
	watcher.watchCert(watcher.opts.CAPath, certPool)
	watcher.watchCert(watcher.opts.ClientCAPath, certPool)
	serverCert, err := tls.LoadX509KeyPair(filepath.Clean(serverCert), filepath.Clean(serverKey))
	require.NoError(t, err)
	assert.Equal(t, &serverCert, watcher.certificate())

	// update the content with bad client certs
	copyFile(t, certFile.Name(), badCaCert)
	copyFile(t, keyFile.Name(), clientKey)

	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Failed to load certificate pair").
				FilterField(zap.String("certificate", certFile.Name())).Len() > 0
		}, "Unable to locate 'Failed to load certificate pair' in log. All logs: %v", logObserver)
	assert.Equal(t, &serverCert, watcher.certificate())
}

func TestReload_err_watch(t *testing.T) {
	opts := Options{
		CAPath: "doesnotexists",
	}
	zcore, logObserver := observer.New(zapcore.InfoLevel)
	watcher, _ := newCertWatcher(opts, zap.New(zcore))
	watcher.watchCert(watcher.opts.CAPath, x509.NewCertPool())
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Cannot set up watcher for certificate").
				FilterField(zap.String("certificate", watcher.opts.CAPath)).Len() > 0
		}, "Unable to locate 'Cannot set up watcher for certificate' in log. All logs: %v", logObserver)
}

func TestReload_kubernetes_secret_update(t *testing.T) {
	mountDir, err := os.MkdirTemp("", "secret-mountpoint")
	require.NoError(t, err)
	defer os.RemoveAll(mountDir)

	// Create directory layout before update:
	//
	// /secret-mountpoint/ca.crt                # symbolic link to ..data/ca.crt
	// /secret-mountpoint/tls.crt               # symbolic link to ..data/tls.crt
	// /secret-mountpoint/tls.key               # symbolic link to ..data/tls.key
	// /secret-mountpoint/..data                # symbolic link to ..timestamp-1
	// /secret-mountpoint/..timestamp-1         # directory
	// /secret-mountpoint/..timestamp-1/ca.crt  # initial version of ca.crt
	// /secret-mountpoint/..timestamp-1/tls.crt # initial version of tls.crt
	// /secret-mountpoint/..timestamp-1/tls.key # initial version of tls.key

	err = os.Symlink("..timestamp-1", filepath.Join(mountDir, "..data"))
	require.NoError(t, err)
	err = os.Symlink(filepath.Join("..data", "ca.crt"), filepath.Join(mountDir, "ca.crt"))
	require.NoError(t, err)
	err = os.Symlink(filepath.Join("..data", "tls.crt"), filepath.Join(mountDir, "tls.crt"))
	require.NoError(t, err)
	err = os.Symlink(filepath.Join("..data", "tls.key"), filepath.Join(mountDir, "tls.key"))
	require.NoError(t, err)

	timestamp1Dir := filepath.Join(mountDir, "..timestamp-1")
	createTimestampDir(t, timestamp1Dir, caCert, serverCert, serverKey)

	opts := Options{
		CAPath:       filepath.Join(mountDir, "ca.crt"),
		ClientCAPath: filepath.Join(mountDir, "ca.crt"),
		CertPath:     filepath.Join(mountDir, "tls.crt"),
		KeyPath:      filepath.Join(mountDir, "tls.key"),
	}

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	watcher, err := newCertWatcher(opts, logger)
	require.NoError(t, err)
	defer watcher.Close()

	certPool := x509.NewCertPool()
	require.NoError(t, err)
	watcher.watchCertPair()
	watcher.watchCert(watcher.opts.CAPath, certPool)
	watcher.watchCert(watcher.opts.ClientCAPath, certPool)

	expectedCert, err := tls.LoadX509KeyPair(serverCert, serverKey)
	require.NoError(t, err)

	assert.Equal(t, expectedCert.Certificate, watcher.certificate().Certificate,
		"certificate should be updated: %v", logObserver.All())

	// After the update, the directory looks like following:
	//
	// /secret-mountpoint/ca.crt                # symbolic link to ..data/ca.crt
	// /secret-mountpoint/tls.crt               # symbolic link to ..data/tls.crt
	// /secret-mountpoint/tls.key               # symbolic link to ..data/tls.key
	// /secret-mountpoint/..data                # symbolic link to ..timestamp-2
	// /secret-mountpoint/..timestamp-2         # new directory
	// /secret-mountpoint/..timestamp-2/ca.crt  # new version of ca.crt
	// /secret-mountpoint/..timestamp-2/tls.crt # new version of tls.crt
	// /secret-mountpoint/..timestamp-2/tls.key # new version of tls.key
	logObserver.TakeAll()

	timestamp2Dir := filepath.Join(mountDir, "..timestamp-2")
	createTimestampDir(t, timestamp2Dir, caCert, clientCert, clientKey)

	err = os.Symlink("..timestamp-2", filepath.Join(mountDir, "..data_tmp"))
	require.NoError(t, err)

	os.Rename(filepath.Join(mountDir, "..data_tmp"), filepath.Join(mountDir, "..data"))
	require.NoError(t, err)
	err = os.RemoveAll(timestamp1Dir)
	require.NoError(t, err)

	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Loaded modified certificate").
				FilterField(zap.String("certificate", opts.CertPath)).Len() > 0
		},
		"Unable to locate 'Loaded modified certificate' in log. All logs: %v", logObserver)

	expectedCert, err = tls.LoadX509KeyPair(clientCert, clientKey)
	require.NoError(t, err)
	assert.Equal(t, expectedCert.Certificate, watcher.certificate().Certificate,
		"certificate should be updated: %v", logObserver.All())

	// Make third update to make sure that the watcher is still working.
	logObserver.TakeAll()

	timestamp3Dir := filepath.Join(mountDir, "..timestamp-3")
	createTimestampDir(t, timestamp3Dir, caCert, serverCert, serverKey)
	err = os.Symlink("..timestamp-3", filepath.Join(mountDir, "..data_tmp"))
	require.NoError(t, err)
	os.Rename(filepath.Join(mountDir, "..data_tmp"), filepath.Join(mountDir, "..data"))
	require.NoError(t, err)
	err = os.RemoveAll(timestamp2Dir)
	require.NoError(t, err)

	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Loaded modified certificate").
				FilterField(zap.String("certificate", opts.CertPath)).Len() > 0
		},
		"Unable to locate 'Loaded modified certificate' in log. All logs: %v", logObserver)

	expectedCert, err = tls.LoadX509KeyPair(serverCert, serverKey)
	require.NoError(t, err)
	assert.Equal(t, expectedCert.Certificate, watcher.certificate().Certificate,
		"certificate should be updated: %v", logObserver.All())
}

func createTimestampDir(t *testing.T, dir string, ca, cert, key string) {
	t.Helper()
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)

	data, err := os.ReadFile(ca)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "ca.crt"), data, 0o600)
	require.NoError(t, err)
	data, err = os.ReadFile(cert)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "tls.crt"), data, 0o600)
	require.NoError(t, err)
	data, err = os.ReadFile(key)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "tls.key"), data, 0o600)
	require.NoError(t, err)
}

func TestAddCertsToWatch_err(t *testing.T) {
	tests := []struct {
		opts Options
	}{
		{
			opts: Options{
				CAPath: "doesnotexists",
			},
		},
		{
			opts: Options{
				CAPath:       caCert,
				ClientCAPath: "doesnotexists",
			},
		},
		{
			opts: Options{
				CAPath:       caCert,
				ClientCAPath: caCert,
				CertPath:     "doesnotexists",
			},
		},
		{
			opts: Options{
				CAPath:       caCert,
				ClientCAPath: caCert,
				CertPath:     serverCert,
				KeyPath:      "doesnotexists",
			},
		},
	}
	for _, test := range tests {
		zcore, logObserver := observer.New(zapcore.InfoLevel)
		watcher, _ := newCertWatcher(test.opts, zap.New(zcore))
		certPool := x509.NewCertPool()
		watcher.watchCertPair()
		watcher.watchCert(watcher.opts.CAPath, certPool)
		watcher.watchCert(watcher.opts.ClientCAPath, certPool)
		assertLogs(t,
			func() bool {
				return logObserver.FilterMessage("Cannot set up watcher for certificate").Len() > 0
			}, "Unable to locate 'Cannot set up watcher for certificate' in log. All logs: %v", logObserver)
	}
}

func TestAddCertsToWatch_remove_ca(t *testing.T) {
	caFile, caFileCloseFn := copyToTempFile(t, "cert.crt", caCert)
	defer caFileCloseFn()
	clientCaFile, clientCaFileClostFn := copyToTempFile(t, "key.crt", caCert)
	defer clientCaFileClostFn()

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	opts := Options{
		CAPath:       caFile.Name(),
		ClientCAPath: clientCaFile.Name(),
	}
	watcher, err := newCertWatcher(opts, logger)
	require.NoError(t, err)
	defer watcher.Close()

	certPool := x509.NewCertPool()
	require.NoError(t, err)
	watcher.watchCert(watcher.opts.CAPath, certPool)
	watcher.watchCert(watcher.opts.ClientCAPath, certPool)

	require.NoError(t, os.Remove(caFile.Name()))
	require.NoError(t, os.Remove(clientCaFile.Name()))
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Unable to read the file").Len() >= 2
		},
		"Unable to locate 'Unable to read the file' in log. All logs: %v", logObserver)
	assert.True(t, logObserver.FilterMessage("Unable to read the file").FilterField(zap.String("file", caFile.Name())).Len() > 0)
	assert.True(t, logObserver.FilterMessage("Unable to read the file").FilterField(zap.String("file", clientCaFile.Name())).Len() > 0)
}

type delayedFormat struct {
	fn func() interface{}
}

func (df delayedFormat) String() string {
	return fmt.Sprintf("%v", df.fn())
}

func assertLogs(t *testing.T, f func() bool, errorMsg string, logObserver *observer.ObservedLogs) {
	assert.Eventuallyf(t, f,
		10*time.Second, 10*time.Millisecond,
		errorMsg,
		delayedFormat{
			fn: func() interface{} { return logObserver.All() },
		},
	)
}

// syncWrite ensures data is written to the given filename and flushed to disk.
// This ensures that any watchers looking for file system changes can be reliably alerted.
func syncWrite(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

func TestReload_err_ca_cert_update(t *testing.T) {
	// copy certs to temp so we can modify them
	caFile, caFileCloseFn := copyToTempFile(t, "cert.crt", caCert)
	defer caFileCloseFn()
	clientCaFile, clientCaFileClostFn := copyToTempFile(t, "key.crt", caCert)
	defer clientCaFileClostFn()

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	opts := Options{
		CAPath:       caFile.Name(),
		ClientCAPath: clientCaFile.Name(),
	}
	watcher, err := newCertWatcher(opts, logger)
	require.NoError(t, err)
	defer watcher.Close()

	certPool := x509.NewCertPool()
	require.NoError(t, err)
	watcher.watchCert(watcher.opts.CAPath, certPool)
	watcher.watchCert(watcher.opts.ClientCAPath, certPool)

	// update the content with bad certs.
	copyFile(t, caFile.Name(), badCaCert)
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Failed to load certificate").
				FilterField(zap.String("certificate", caFile.Name())).Len() > 0
		},
		"Unable to locate 'certificate' in log. All logs: %v", logObserver)

	copyFile(t, clientCaFile.Name(), badCaCert)
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Failed to load certificate").
				FilterField(zap.String("certificate", clientCaFile.Name())).Len() > 0
		},
		"Unable to locate 'Failed to load certificate' in log. All logs: %v", logObserver)
}

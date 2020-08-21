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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
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

	caCert    = "./testdata/example-CA-cert.pem"
	badCaCert = "./testdata/bad-CA-cert.txt"
)

func TestReload(t *testing.T) {
	// copy certs to temp so we can modify them
	certFile, err := ioutil.TempFile("", "cert.crt")
	require.NoError(t, err)
	defer os.Remove(certFile.Name())
	certData, err := ioutil.ReadFile(serverCert)
	require.NoError(t, err)
	_, err = certFile.Write(certData)
	require.NoError(t, err)
	certFile.Close()

	keyFile, err := ioutil.TempFile("", "key.crt")
	require.NoError(t, err)
	defer os.Remove(keyFile.Name())
	keyData, err := ioutil.ReadFile(serverKey)
	require.NoError(t, err)
	_, err = keyFile.Write(keyData)
	require.NoError(t, err)
	keyFile.Close()

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
	go watcher.watchChangesLoop(certPool, certPool)
	cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
	require.NoError(t, err)
	assert.Equal(t, &cert, watcher.certificate())

	// update the content with client certs
	certData, err = ioutil.ReadFile(clientCert)
	require.NoError(t, err)
	err = ioutil.WriteFile(certFile.Name(), certData, 0644)
	require.NoError(t, err)
	keyData, err = ioutil.ReadFile(clientKey)
	require.NoError(t, err)
	err = ioutil.WriteFile(keyFile.Name(), keyData, 0644)
	require.NoError(t, err)

	waitUntil(func() bool {
		return logObserver.FilterField(zap.String("certificate", certFile.Name())).Len() > 0
	}, 100, time.Millisecond*200)
	assert.True(t, logObserver.FilterField(zap.String("certificate", certFile.Name())).Len() > 0)

	cert, err = tls.LoadX509KeyPair(filepath.Clean(clientCert), clientKey)
	require.NoError(t, err)
	assert.Equal(t, &cert, watcher.certificate())
}

func TestReload_ca_certs(t *testing.T) {
	// copy certs to temp so we can modify them
	caFile, err := ioutil.TempFile("", "cert.crt")
	require.NoError(t, err)
	defer os.Remove(caFile.Name())
	caData, err := ioutil.ReadFile(caCert)
	require.NoError(t, err)
	_, err = caFile.Write(caData)
	require.NoError(t, err)
	caFile.Close()

	clientCaFile, err := ioutil.TempFile("", "key.crt")
	require.NoError(t, err)
	defer os.Remove(clientCaFile.Name())
	clientCaData, err := ioutil.ReadFile(caCert)
	require.NoError(t, err)
	_, err = clientCaFile.Write(clientCaData)
	require.NoError(t, err)
	clientCaFile.Close()

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
	go watcher.watchChangesLoop(certPool, certPool)

	// update the content with client certs
	caData, err = ioutil.ReadFile(caCert)
	require.NoError(t, err)
	err = ioutil.WriteFile(caFile.Name(), caData, 0644)
	require.NoError(t, err)
	clientCaData, err = ioutil.ReadFile(caCert)
	require.NoError(t, err)
	err = ioutil.WriteFile(clientCaFile.Name(), clientCaData, 0644)
	require.NoError(t, err)

	waitUntil(func() bool {
		return logObserver.FilterField(zap.String("certificate", caFile.Name())).Len() > 0
	}, 100, time.Millisecond*200)
	assert.True(t, logObserver.FilterField(zap.String("certificate", caFile.Name())).Len() > 0)

	waitUntil(func() bool {
		return logObserver.FilterField(zap.String("certificate", clientCaFile.Name())).Len() > 0
	}, 100, time.Millisecond*200)
	assert.True(t, logObserver.FilterField(zap.String("certificate", clientCaFile.Name())).Len() > 0)

}

func TestReload_err_cert_update(t *testing.T) {
	// copy certs to temp so we can modify them
	certFile, err := ioutil.TempFile("", "cert.crt")
	require.NoError(t, err)
	defer os.Remove(certFile.Name())
	certData, err := ioutil.ReadFile(serverCert)
	require.NoError(t, err)
	_, err = certFile.Write(certData)
	require.NoError(t, err)
	certFile.Close()

	keyFile, err := ioutil.TempFile("", "key.crt")
	require.NoError(t, err)
	defer os.Remove(keyFile.Name())
	keyData, err := ioutil.ReadFile(serverKey)
	require.NoError(t, err)
	_, err = keyFile.Write(keyData)
	require.NoError(t, err)
	keyFile.Close()

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
	go watcher.watchChangesLoop(certPool, certPool)
	serverCert, err := tls.LoadX509KeyPair(filepath.Clean(serverCert), filepath.Clean(serverKey))
	require.NoError(t, err)
	assert.Equal(t, &serverCert, watcher.certificate())

	// update the content with client certs
	certData, err = ioutil.ReadFile(badCaCert)
	require.NoError(t, err)
	err = ioutil.WriteFile(certFile.Name(), certData, 0644)
	require.NoError(t, err)
	keyData, err = ioutil.ReadFile(clientKey)
	require.NoError(t, err)
	err = ioutil.WriteFile(keyFile.Name(), keyData, 0644)
	require.NoError(t, err)

	waitUntil(func() bool {
		return logObserver.FilterMessage("Failed to load certificate").Len() > 0
	}, 100, time.Millisecond*200)
	assert.True(t, logObserver.FilterField(zap.String("certificate", certFile.Name())).Len() > 0)
	assert.Equal(t, &serverCert, watcher.certificate())
}

func TestReload_err_watch(t *testing.T) {
	opts := Options{
		CAPath: "doesnotexists",
	}
	watcher, err := newCertWatcher(opts, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
	assert.Nil(t, watcher)
}

func TestAddCertsToWatch_err(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)

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
		err := addCertsToWatch(watcher, test.opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such file or directory")
	}
}

func TestAddCertsToWatch_remove_ca(t *testing.T) {
	caFile, err := ioutil.TempFile("", "ca.crt")
	require.NoError(t, err)
	defer os.Remove(caFile.Name())
	caData, err := ioutil.ReadFile(caCert)
	require.NoError(t, err)
	_, err = caFile.Write(caData)
	require.NoError(t, err)
	caFile.Close()

	clientCaFile, err := ioutil.TempFile("", "clientCa.crt")
	require.NoError(t, err)
	defer os.Remove(clientCaFile.Name())
	clientCaData, err := ioutil.ReadFile(caCert)
	require.NoError(t, err)
	_, err = clientCaFile.Write(clientCaData)
	require.NoError(t, err)
	clientCaFile.Close()

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
	go watcher.watchChangesLoop(certPool, certPool)

	require.NoError(t, os.Remove(caFile.Name()))
	require.NoError(t, os.Remove(clientCaFile.Name()))
	waitUntil(func() bool {
		return logObserver.FilterMessage("Certificate has been removed, using the last known version").Len() > 0
	}, 100, time.Millisecond*100)
	assert.True(t, logObserver.FilterMessage("Certificate has been removed, using the last known version").FilterField(zap.String("certificate", caFile.Name())).Len() > 0)
	assert.True(t, logObserver.FilterMessage("Certificate has been removed, using the last known version").FilterField(zap.String("certificate", clientCaFile.Name())).Len() > 0)
}

func waitUntil(f func() bool, iterations int, sleepInterval time.Duration) {
	for i := 0; i < iterations; i++ {
		if f() {
			return
		}
		time.Sleep(sleepInterval)
	}
}

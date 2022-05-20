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

package flags

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/ports"
)

var testCertKeyLocation = "../../pkg/config/tlscfg/testdata"

func TestAdminServerHandlesPortZero(t *testing.T) {
	adminServer := NewAdminServer(":0")

	v, _ := config.Viperize(adminServer.AddFlags)

	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	adminServer.initFromViper(v, logger)

	assert.NoError(t, adminServer.Serve())
	defer adminServer.Close()

	message := logs.FilterMessage("Admin server started")
	assert.Equal(t, 1, message.Len(), "Expected Admin server started log message.")

	onlyEntry := message.All()[0]
	hostPort := onlyEntry.ContextMap()["http.host-port"].(string)
	port, _ := strconv.Atoi(strings.Split(hostPort, ":")[3])
	assert.Greater(t, port, 0)
}

func TestCollectorAdminWithFailedFlags(t *testing.T) {
	adminServer := NewAdminServer(fmt.Sprintf(":%d", ports.CollectorAdminHTTP))
	zapCore, _ := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	v, command := config.Viperize(adminServer.AddFlags)
	err := command.ParseFlags([]string{
		"--admin.http.tls.enabled=false",
		"--admin.http.tls.cert=blah", // invalid unless tls.enabled
	})
	require.NoError(t, err)
	err = adminServer.initFromViper(v, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse admin server TLS options")
}

func TestAdminServerTLS(t *testing.T) {
	testCases := []struct {
		name                 string
		serverTLSFlags       []string
		clientTLS            tlscfg.Options
		expectTLSClientErr   bool
		expectAdminClientErr bool
		expectServerFail     bool
	}{
		{
			name: "should fail with TLS client to untrusted TLS server",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				ServerName: "example.com",
			},
			expectTLSClientErr:   true,
			expectAdminClientErr: true,
			expectServerFail:     false,
		},
		{
			name: "should fail with TLS client to trusted TLS server with incorrect hostname",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "nonEmpty",
			},
			expectTLSClientErr:   true,
			expectAdminClientErr: true,
			expectServerFail:     false,
		},
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
			},
			expectTLSClientErr:   false,
			expectAdminClientErr: false,
			expectServerFail:     false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
				"--admin.http.tls.client-ca=" + testCertKeyLocation + "/example-CA-cert.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
			},
			expectTLSClientErr:   false,
			expectServerFail:     false,
			expectAdminClientErr: true,
		},
		{
			name: "should pass with TLS client with cert to trusted TLS server requiring cert",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
				"--admin.http.tls.client-ca=" + testCertKeyLocation + "/example-CA-cert.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:    testCertKeyLocation + "/example-client-key.pem",
			},
			expectTLSClientErr:   false,
			expectServerFail:     false,
			expectAdminClientErr: false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
				"--admin.http.tls.client-ca=" + testCertKeyLocation + "/wrong-CA-cert.pem", // NB: wrong CA
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:    testCertKeyLocation + "/example-client-key.pem",
			},
			expectTLSClientErr:   false,
			expectServerFail:     false,
			expectAdminClientErr: true,
		},
		{
			name: "should fail with TLS client with cert to trusted TLS server with incorrect TLS min",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
				"--admin.http.tls.client-ca=" + testCertKeyLocation + "/example-CA-cert.pem",
				"--admin.http.tls.min-version=1.5",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:    testCertKeyLocation + "/example-client-key.pem",
			},
			expectTLSClientErr:   true,
			expectServerFail:     true,
			expectAdminClientErr: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			adminServer := NewAdminServer(fmt.Sprintf(":%d", ports.CollectorAdminHTTP))

			v, command := config.Viperize(adminServer.AddFlags)
			err := command.ParseFlags(test.serverTLSFlags)
			require.NoError(t, err)
			zapCore, _ := observer.New(zap.InfoLevel)
			logger := zap.New(zapCore)

			err = adminServer.initFromViper(v, logger)

			if test.expectServerFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			adminServer.Serve()
			defer adminServer.Close()

			clientTLSCfg, err0 := test.clientTLS.Config(zap.NewNop())
			require.NoError(t, err0)
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			conn, clientError := tls.DialWithDialer(dialer, "tcp", fmt.Sprintf("localhost:%d", ports.CollectorAdminHTTP), clientTLSCfg)

			if test.expectTLSClientErr {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
				require.Nil(t, conn.Close())
			}

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: clientTLSCfg,
				},
			}

			response, requestError := client.Get(fmt.Sprintf("https://localhost:%d", ports.CollectorAdminHTTP))

			if test.expectAdminClientErr {
				require.Error(t, requestError)
			} else {
				require.NoError(t, requestError)
				require.NotNil(t, response)
			}
		})
	}
}

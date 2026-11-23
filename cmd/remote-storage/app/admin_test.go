// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
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
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/ports"
)

func TestAdminServerHealthCheck(t *testing.T) {
	adminServer := NewAdminServer()

	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	adminServer.configure(AdminServerConfig{Endpoint: ":0"}, logger)
	require.NoError(t, adminServer.Serve())
	defer adminServer.Close()

	// Get the actual address from the log
	message := logs.FilterMessage("Admin server started")
	require.Equal(t, 1, message.Len())
	hostPort := message.All()[0].ContextMap()["http.host-port"].(string)

	// Health check should initially be unavailable (503)
	resp, err := http.Get(fmt.Sprintf("http://%s/", hostPort))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	// Set to ready - should return 204
	adminServer.Host().Ready()
	resp, err = http.Get(fmt.Sprintf("http://%s/", hostPort))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Set to unavailable - should return 503
	adminServer.Host().SetUnavailable()
	resp, err = http.Get(fmt.Sprintf("http://%s/", hostPort))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestAdminServerHandlesPortZero(t *testing.T) {
	adminServer := NewAdminServer()

	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	adminServer.configure(AdminServerConfig{Endpoint: ":0"}, logger)

	require.NoError(t, adminServer.Serve())
	defer adminServer.Close()

	message := logs.FilterMessage("Admin server started")
	assert.Equal(t, 1, message.Len(), "Expected Admin server started log message.")

	onlyEntry := message.All()[0]
	hostPort := onlyEntry.ContextMap()["http.host-port"].(string)
	port, _ := strconv.Atoi(strings.Split(hostPort, ":")[3])
	assert.Positive(t, port)
}

func TestAdminServerConfigure(t *testing.T) {
	adminServer := NewAdminServer()
	adminServer.configure(AdminServerConfig{Endpoint: ":1"}, zap.NewNop())
	assert.Equal(t, ":1", adminServer.serverCfg.NetAddr.Endpoint)
	assert.Equal(t, confignet.TransportTypeTCP, adminServer.serverCfg.NetAddr.Transport)
	assert.False(t, adminServer.serverCfg.TLS.HasValue())
}

func TestAdminServerTLS(t *testing.T) {
	testCases := []struct {
		name      string
		serverTLS configtls.ServerConfig
		clientTLS configtls.ClientConfig
	}{
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			serverTLS: configtls.ServerConfig{
				Config: configtls.Config{
					CertFile: testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-server-key.pem",
				},
			},
			clientTLS: configtls.ClientConfig{
				Insecure: false,
				Config: configtls.Config{
					CAFile: testCertKeyLocation + "/example-CA-cert.pem",
				},
				ServerName: "example.com",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			adminServer := NewAdminServer()
			adminServer.configure(AdminServerConfig{
				Endpoint: fmt.Sprintf(":%d", ports.RemoteStorageAdminHTTP),
				TLS:      configoptional.Some(test.serverTLS),
			}, zaptest.NewLogger(t))

			require.NoError(t, adminServer.Serve())
			defer adminServer.Close()

			clientTLSCfg, err0 := test.clientTLS.LoadTLSConfig(context.Background())
			require.NoError(t, err0)
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			conn, clientError := tls.DialWithDialer(dialer, "tcp", fmt.Sprintf("localhost:%d", ports.RemoteStorageAdminHTTP), clientTLSCfg)
			require.NoError(t, clientError)
			require.NoError(t, conn.Close())

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: clientTLSCfg,
				},
			}
			url := fmt.Sprintf("https://localhost:%d", ports.RemoteStorageAdminHTTP)
			req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
			require.NoError(t, err)
			req.Close = true // avoid persistent connections which leak goroutines
			response, requestError := client.Do(req)
			require.NoError(t, requestError)
			defer response.Body.Close()
			require.NotNil(t, response)
		})
	}
}

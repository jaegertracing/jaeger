// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

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
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/ports"
)

var testCertKeyLocation = "../../../internal/config/tlscfg/testdata"

func TestAdminServerHealthCheck(t *testing.T) {
	adminServer := NewAdminServer(":0")

	v, _ := config.Viperize(adminServer.AddFlags)
	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	require.NoError(t, adminServer.initFromViper(v, logger))
	require.NoError(t, adminServer.Serve())
	defer adminServer.Close()

	// Get the actual address from the log
	message := logs.FilterMessage("Admin server started")
	require.Equal(t, 1, message.Len())
	hostPort := message.All()[0].ContextMap()["http.host-port"].(string)

	// Health check should initially be unavailable
	assert.Equal(t, Unavailable, adminServer.HC().Get())

	// Set to ready
	adminServer.HC().Ready()
	assert.Equal(t, Ready, adminServer.HC().Get())

	// Verify HTTP endpoint returns correct status
	resp, err := http.Get(fmt.Sprintf("http://%s/", hostPort))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Set to unavailable
	adminServer.HC().SetUnavailable()
	assert.Equal(t, Unavailable, adminServer.HC().Get())

	// Verify HTTP endpoint returns 503
	resp, err = http.Get(fmt.Sprintf("http://%s/", hostPort))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestAdminServerHandlesPortZero(t *testing.T) {
	adminServer := NewAdminServer(":0")

	v, _ := config.Viperize(adminServer.AddFlags)

	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	adminServer.initFromViper(v, logger)

	require.NoError(t, adminServer.Serve())
	defer adminServer.Close()

	message := logs.FilterMessage("Admin server started")
	assert.Equal(t, 1, message.Len(), "Expected Admin server started log message.")

	onlyEntry := message.All()[0]
	hostPort := onlyEntry.ContextMap()["http.host-port"].(string)
	port, _ := strconv.Atoi(strings.Split(hostPort, ":")[3])
	assert.Positive(t, port)
}

func TestAdminWithFailedFlags(t *testing.T) {
	adminServer := NewAdminServer(fmt.Sprintf(":%d", ports.RemoteStorageAdminHTTP))
	zapCore, _ := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	v, command := config.Viperize(adminServer.AddFlags)
	err := command.ParseFlags([]string{
		"--admin.http.tls.enabled=false",
		"--admin.http.tls.cert=blah", // invalid unless tls.enabled
	})
	require.NoError(t, err)
	err = adminServer.initFromViper(v, logger)
	assert.ErrorContains(t, err, "failed to parse admin server TLS options")
}

func TestAdminServerTLS(t *testing.T) {
	testCases := []struct {
		name           string
		serverTLSFlags []string
		clientTLS      configtls.ClientConfig
	}{
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			serverTLSFlags: []string{
				"--admin.http.tls.enabled=true",
				"--admin.http.tls.cert=" + testCertKeyLocation + "/example-server-cert.pem",
				"--admin.http.tls.key=" + testCertKeyLocation + "/example-server-key.pem",
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
			adminServer := NewAdminServer(fmt.Sprintf(":%d", ports.RemoteStorageAdminHTTP))

			v, command := config.Viperize(adminServer.AddFlags)
			err := command.ParseFlags(test.serverTLSFlags)
			require.NoError(t, err)

			err = adminServer.initFromViper(v, zaptest.NewLogger(t))
			require.NoError(t, err)

			adminServer.Serve()
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

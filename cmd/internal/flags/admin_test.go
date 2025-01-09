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

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/ports"
)

var testCertKeyLocation = "../../../pkg/config/tlscfg/testdata"

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

func TestAdminHealthCheck(t *testing.T) {
	adminServer := NewAdminServer(":0")
	status := adminServer.HC().Get()
	assert.Equal(t, healthcheck.Unavailable, status)
}

func TestAdminFailToServe(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	l.Close() // forcing Serve on a closed connection

	adminServer := NewAdminServer(":0")
	v, command := config.Viperize(adminServer.AddFlags)
	command.ParseFlags([]string{})
	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	require.NoError(t, adminServer.initFromViper(v, logger))

	adminServer.serveWithListener(l)
	t.Cleanup(func() { assert.NoError(t, adminServer.Close()) })

	waitForEqual(t, healthcheck.Broken, func() any { return adminServer.HC().Get() })

	logEntries := logs.TakeAll()
	var matchedEntry string
	for _, log := range logEntries {
		if strings.Contains(log.Message, "failed to serve") {
			matchedEntry = log.Message
			break
		}
	}
	assert.Contains(t, matchedEntry, "failed to serve")
}

func TestAdminWithFailedFlags(t *testing.T) {
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
			adminServer := NewAdminServer(fmt.Sprintf(":%d", ports.CollectorAdminHTTP))

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
			conn, clientError := tls.DialWithDialer(dialer, "tcp", fmt.Sprintf("localhost:%d", ports.CollectorAdminHTTP), clientTLSCfg)
			require.NoError(t, clientError)
			require.NoError(t, conn.Close())

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: clientTLSCfg,
				},
			}
			url := fmt.Sprintf("https://localhost:%d", ports.CollectorAdminHTTP)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)
			req.Close = true // avoid persistent connections which leak goroutines
			response, requestError := client.Do(req)
			require.NoError(t, requestError)
			defer response.Body.Close()
			require.NotNil(t, response)
		})
	}
}

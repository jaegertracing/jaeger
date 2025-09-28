// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ClickHouse/ch-go/proto"
	"github.com/ClickHouse/clickhouse-go/v2"
	chproto "github.com/ClickHouse/clickhouse-go/v2/lib/proto"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	pingQuery      = "SELECT 1"
	handshakeQuery = "SELECT displayName(), version(), revision(), timezone()"
)

func newMockClickHouseServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		query := string(body)

		block := chproto.NewBlock()

		switch query {
		case pingQuery:
			block.AddColumn("1", "UInt8")
			block.Append(uint8(1))
		case handshakeQuery:
			block.AddColumn("displayName()", "String")
			block.AddColumn("version()", "String")
			block.AddColumn("revision()", "UInt32")
			block.AddColumn("timezone()", "String")
			block.Append("mock-server", "23.3.1", chproto.DBMS_MIN_REVISION_WITH_CUSTOM_SERIALIZATION, "UTC")
		default:
		}

		var buf proto.Buffer
		block.Encode(&buf, clickhouse.ClientTCPProtocolVersion)

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(buf.Buf)
	}))
}

func TestFactory(t *testing.T) {
	srv := newMockClickHouseServer()
	defer srv.Close()

	cfg := Configuration{
		Protocol: "http",
		Addresses: []string{
			srv.Listener.Addr().String(),
		},
		Database: "default",
		Auth: Authentication{
			Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
				Username: "user",
				Password: "password",
			}),
		},
	}

	f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
	require.NoError(t, err)
	require.NotNil(t, f)

	tr, err := f.CreateTraceReader()
	require.NoError(t, err)
	require.NotNil(t, tr)

	tw, err := f.CreateTraceWriter()
	require.NoError(t, err)
	require.NotNil(t, tw)

	require.NoError(t, f.Close())
}

func TestFactory_PingError(t *testing.T) {
	srv := newMockClickHouseServer()
	defer srv.Close()

	cfg := Configuration{
		Protocol: "http",
		Addresses: []string{
			"127.0.0.1:9999", // wrong address to simulate ping error
		},
		DialTimeout: 1 * time.Second,
	}

	f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
	require.ErrorContains(t, err, "failed to ping ClickHouse")
	require.Nil(t, f)
}

func TestGetProtocol(t *testing.T) {
	tests := []struct {
		protocol string
		expected clickhouse.Protocol
	}{
		{
			protocol: "http",
			expected: clickhouse.HTTP,
		},
		{
			protocol: "native",
			expected: clickhouse.Native,
		},
		{
			protocol: "",
			expected: clickhouse.Native,
		},
		{
			protocol: "unknown",
			expected: clickhouse.Native,
		},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			result := getProtocol(tt.protocol)
			require.Equal(t, tt.expected, result)
		})
	}
}

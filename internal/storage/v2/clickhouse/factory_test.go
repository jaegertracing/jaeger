// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"
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

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	pingQuery      = "SELECT 1"
	handshakeQuery = "SELECT displayName(), version(), revision(), timezone()"
)

type mockFailureConfig map[string]error

func newMockClickHouseServer(failures mockFailureConfig) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		query := string(body)

		block := chproto.NewBlock()

		if err, shouldFail := failures[query]; shouldFail {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

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
	tests := []struct {
		name         string
		createSchema bool
	}{
		{
			name:         "without schema creation",
			createSchema: false,
		},
		{
			name:         "with schema creation",
			createSchema: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMockClickHouseServer(mockFailureConfig{})
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
		})
	}
}

func TestNewFactory_Errors(t *testing.T) {
	tests := []struct {
		name          string
		failureConfig mockFailureConfig
		expectedError string
	}{
		{
			name: "ping error",
			failureConfig: mockFailureConfig{
				pingQuery: errors.New("ping error"),
			},
			expectedError: "failed to ping ClickHouse",
		},
		{
			name: "spans table creation error",
			failureConfig: mockFailureConfig{
				sql.CreateSpansTable: errors.New("spans table creation error"),
			},
			expectedError: "failed to create spans table",
		},
		{
			name: "services table creation error",
			failureConfig: mockFailureConfig{
				sql.CreateServicesTable: errors.New("services table creation error"),
			},
			expectedError: "failed to create services table",
		},
		{
			name: "services materialized view creation error",
			failureConfig: mockFailureConfig{
				sql.CreateServicesMaterializedView: errors.New("services materialized view creation error"),
			},
			expectedError: "failed to create services materialized view",
		},
		{
			name: "operations table creation error",
			failureConfig: mockFailureConfig{
				sql.CreateOperationsTable: errors.New("operations table creation error"),
			},
			expectedError: "failed to create operations table",
		},
		{
			name: "operations materialized view creation error",
			failureConfig: mockFailureConfig{
				sql.CreateOperationsMaterializedView: errors.New("operations materialized view creation error"),
			},
			expectedError: "failed to create operations materialized view",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMockClickHouseServer(tt.failureConfig)
			defer srv.Close()

			cfg := Configuration{
				Protocol: "http",
				Addresses: []string{
					srv.Listener.Addr().String(),
				},
				DialTimeout:  1 * time.Second,
				CreateSchema: true,
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
			require.ErrorContains(t, err, tt.expectedError)
			require.Nil(t, f)
		})
	}
}

func TestPurge(t *testing.T) {
	tests := []struct {
		name          string
		failureConfig mockFailureConfig
		expectedError string
	}{
		{
			name: "truncate spans table error",
			failureConfig: mockFailureConfig{
				sql.TruncateSpans: errors.New("truncate spans table error"),
			},
			expectedError: "failed to purge spans",
		},
		{
			name: "truncate services table error",
			failureConfig: mockFailureConfig{
				sql.TruncateServices: errors.New("truncate services table error"),
			},
			expectedError: "failed to purge services",
		},
		{
			name: "truncate operations table error",
			failureConfig: mockFailureConfig{
				sql.TruncateOperations: errors.New("truncate operations table error"),
			},
			expectedError: "failed to purge operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMockClickHouseServer(tt.failureConfig)
			defer srv.Close()

			cfg := Configuration{
				Protocol: "http",
				Addresses: []string{
					srv.Listener.Addr().String(),
				},
				DialTimeout:  1 * time.Second,
				CreateSchema: true,
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, f.Close())
			})

			err = f.Purge(context.Background())
			require.ErrorContains(t, err, tt.expectedError)
		})
	}
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

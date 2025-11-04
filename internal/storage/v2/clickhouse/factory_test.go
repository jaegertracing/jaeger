// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

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
			srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
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

			dr, err := f.CreateDependencyReader()
			require.NoError(t, err)
			require.NotNil(t, dr)

			err = f.Purge(context.Background())
			require.NoError(t, err)

			require.NoError(t, f.Close())
		})
	}
}

func TestNewFactory_Errors(t *testing.T) {
	tests := []struct {
		name          string
		failureConfig clickhousetest.FailureConfig
		expectedError string
	}{
		{
			name: "ping error",
			failureConfig: clickhousetest.FailureConfig{
				clickhousetest.PingQuery: errors.New("ping error"),
			},
			expectedError: "failed to ping ClickHouse",
		},
		{
			name: "spans table creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateSpansTable: errors.New("spans table creation error"),
			},
			expectedError: "failed to create spans table",
		},
		{
			name: "services table creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateServicesTable: errors.New("services table creation error"),
			},
			expectedError: "failed to create services table",
		},
		{
			name: "services materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateServicesMaterializedView: errors.New("services materialized view creation error"),
			},
			expectedError: "failed to create services materialized view",
		},
		{
			name: "operations table creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateOperationsTable: errors.New("operations table creation error"),
			},
			expectedError: "failed to create operations table",
		},
		{
			name: "operations materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateOperationsMaterializedView: errors.New("operations materialized view creation error"),
			},
			expectedError: "failed to create operations materialized view",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := clickhousetest.NewServer(tt.failureConfig)
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
		failureConfig clickhousetest.FailureConfig
		expectedError string
	}{
		{
			name: "truncate spans table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateSpans: errors.New("truncate spans table error"),
			},
			expectedError: "failed to purge spans",
		},
		{
			name: "truncate services table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateServices: errors.New("truncate services table error"),
			},
			expectedError: "failed to purge services",
		},
		{
			name: "truncate operations table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateOperations: errors.New("truncate operations table error"),
			},
			expectedError: "failed to purge operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := clickhousetest.NewServer(tt.failureConfig)
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

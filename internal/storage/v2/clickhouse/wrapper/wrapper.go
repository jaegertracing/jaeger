// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package wrapper

import (
	"context"
	"crypto/tls"

	"github.com/ClickHouse/clickhouse-go/v2"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	ck "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/config"
)

// ConnWrapper holds a clickhouse.Conn instance to be used to get traces from ClickHouse.
type ConnWrapper struct {
	conn clickhouse.Conn
}

// PrepareBatch is used to write traces with batch mode.
func (cw *ConnWrapper) PrepareBatch(ctx context.Context, query string) (client.Batch, error) {
	return cw.conn.PrepareBatch(ctx, query)
}

// Query is used to get traces by taking many query parameters.
func (cw *ConnWrapper) Query(ctx context.Context, query string, args ...any) (client.Rows, error) {
	return cw.conn.Query(ctx, query, args...)
}

// Close the connection.
func (cw *ConnWrapper) Close() error {
	return cw.conn.Close()
}

// WrapOpen takes a given config and returns an abstraction client of clickhouse.Conn.
func WrapOpen(ctx context.Context, cfg ck.Config) (client.Conn, error) {
	tlsConfig, err := toTlsConfig(ctx, cfg.TLS)
	if err != nil {
		return nil, err
	}
	option := clickhouse.Options{
		Addr: cfg.Servers,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Auth.Username,
			Password: cfg.Auth.Password,
		},
		TLS: tlsConfig,
	}

	ackConn, err := clickhouse.Open(&option)
	if err != nil {
		return nil, err
	}
	return &ConnWrapper{conn: ackConn}, nil
}

// toTlsConfig converts OTel TLS configuration to a tls.Config.
func toTlsConfig(ctx context.Context, config configtls.ClientConfig) (*tls.Config, error) {
	if config.Insecure {
		return config.LoadTLSConfig(ctx)
	}
	return nil, nil
}

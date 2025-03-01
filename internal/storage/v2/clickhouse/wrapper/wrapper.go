// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package wrapper

import (
	"context"

	"github.com/ClickHouse/ch-go"
	"github.com/ClickHouse/ch-go/chpool"
	v2Client "github.com/ClickHouse/clickhouse-go/v2"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/conn"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/pool"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/internal"
)

func WrapDial(config pool.Configuration, log *zap.Logger) (client.Pool, error) {
	option := chpool.Options{
		ClientOptions: ch.Options{
			Logger:   log,
			Address:  config.ClientConfig.Address,
			Database: config.ClientConfig.Database,
			User:     config.ClientConfig.Username,
			Password: config.ClientConfig.Password,
		},
		MaxConnLifetime:   config.PoolConfig.MaxConnLifetime,
		MaxConnIdleTime:   config.PoolConfig.MaxConnIdleTime,
		MinConns:          config.PoolConfig.MinConns,
		MaxConns:          config.PoolConfig.MaxConns,
		HealthCheckPeriod: config.PoolConfig.HealthCheckPeriod,
	}

	ackPool, err := chpool.Dial(context.Background(), option)
	if err != nil {
		return PoolWrapper{}, err
	}
	return WarpPool(ackPool), err
}

// PoolWrapper is a wrapper around clickhouse-go used by read path, ch-go used by write path.
type PoolWrapper struct {
	Pool *chpool.Pool
}

// Do calls this function to internal pool.
func (c PoolWrapper) Do(ctx context.Context, query string, td ...ptrace.Traces) error {
	q := ch.Query{Body: query}
	if td != nil {
		q.Input = internal.Input(td[0])
	}
	return c.WrapDo(ctx, q)
}

func (c PoolWrapper) Close() error {
	if c.Pool != nil {
		c.Pool.Close()
	}
	return nil
}

func WarpPool(p *chpool.Pool) PoolWrapper {
	return PoolWrapper{Pool: p}
}

func (c PoolWrapper) WrapDo(ctx context.Context, query ch.Query) error {
	return c.Pool.Do(ctx, query)
}

func WrapOpen(config conn.Configuration) (client.Conn, error) {
	option := v2Client.Options{
		Addr: config.Address,
		Auth: v2Client.Auth{
			Database: config.Database,
			Username: config.Username,
			Password: config.Password,
		},
	}

	ackConn, err := v2Client.Open(&option)
	if err != nil {
		return ConnWrapper{}, err
	}
	return WarpConn(ackConn), nil
}

// ConnWrapper is a wrapper around clickhouse-go used by read path, ch-go used by write path.
type ConnWrapper struct {
	Conn v2Client.Conn
}

func WarpConn(c v2Client.Conn) ConnWrapper {
	return ConnWrapper{Conn: c}
}

// Query calls this function to internal connection.
func (c ConnWrapper) Query(ctx context.Context, query string, args string) (client.Rows, error) {
	return c.WrapQuery(ctx, query, args)
}

func (c ConnWrapper) Exec(ctx context.Context, query string) error {
	return c.WrapExec(ctx, query)
}

// Close closes connection and pool.
func (c ConnWrapper) Close() error {
	if c.Conn != nil {
		err := c.Conn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c ConnWrapper) WrapQuery(ctx context.Context, query string, args string) (client.Rows, error) {
	return c.Conn.Query(ctx, query, args)
}

func (c ConnWrapper) WrapExec(ctx context.Context, query string) error {
	return c.Conn.Exec(ctx, query)
}

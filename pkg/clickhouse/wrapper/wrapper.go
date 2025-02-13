// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package wrapper

import (
	"context"

	"github.com/ClickHouse/ch-go"
	"github.com/ClickHouse/ch-go/chpool"
	v2Client "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/jaegertracing/jaeger/pkg/clickhouse"
	trace "github.com/jaegertracing/jaeger/pkg/clickhouse/internal"
)

// ClientWrapper is a wrapper around clickhouse-go used by read path, ch-go used by write path.
type ClientWrapper struct {
	Conn v2Client.Conn
	Pool *chpool.Pool
}

func WrapCHClient(conn v2Client.Conn, pool *chpool.Pool) ClientWrapper {
	return ClientWrapper{
		Conn: conn,
		Pool: pool,
	}
}

// Do calls this function to internal pool.
func (c ClientWrapper) Do(ctx context.Context, query clickhouse.QueryParam) error {
	return c.WrapDo(ctx, tracesQuery(query))
}

// Close closes connection or pool.
func (c ClientWrapper) Close() error {
	if c.Pool != nil {
		c.Pool.Close()
	}
	if c.Conn != nil {
		err := c.Conn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c ClientWrapper) WrapDo(ctx context.Context, query ch.Query) error {
	return c.Pool.Do(ctx, query)
}

func tracesQuery(query clickhouse.QueryParam) ch.Query {
	return ch.Query{
		Body:  query.Body,
		Input: trace.Input(query.Input),
	}
}

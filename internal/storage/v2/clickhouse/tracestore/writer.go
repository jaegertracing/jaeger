// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

const insertTrace = "INSERT INTO traces"

type Writer struct {
	client clickhouse.Conn
}

func (tw *Writer) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	var err error
	batch, err := tw.client.PrepareBatch(ctx, insertTrace)
	if err != nil {
		return fmt.Errorf("error closing ClickHouse batch: %w", err)
	}

	// The dbmodel.ToDBModel is now based on the ch-go library.
	// It will be deprecated in favor of a new function based on the clickhouse-go library.
	// For more information, see https://github.com/jaegertracing/jaeger/pull/7149.
	traces := dbmodel.ToDBModel(td)

	for range traces {
		// TODO Maybe AppendStruct is a better choice.
		err := batch.Append()
		if err != nil {
			return fmt.Errorf("failed to append trace data to ClickHouse batch: %w", err)
		}
	}
	err = batch.Send()
	if err != nil {
		return fmt.Errorf("failed to send data to Clickhouse: %w", err)
	}
	return nil
}

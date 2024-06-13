// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.18.0"
)

const (
	insertSpansSQL = `INSERT INTO %s (
		Timestamp,
		TraceId,
		SpanId,
		ParentSpanId,
		Operation,
		Service,
		Tags.keys,
		Tags.values,
		Duration
		) VALUES (
				  ?,
				  ?,
				  ?,
				  ?,
				  ?,
				  ?,
				  ?,
				  ?,
				  ?,
				  )
	`
)

func ExportSpans(ctx context.Context, db *sql.DB, tableName string, td ptrace.Traces) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		tx.Rollback()
	}()

	if err = insertSpans(ctx, tx, tableName, td); err != nil {
		return err
	}

	return tx.Commit()
}

func insertSpans(ctx context.Context, tx *sql.Tx, tableName string, td ptrace.Traces) error {
	statement, err := tx.PrepareContext(ctx, fmt.Sprintf(insertSpansSQL, tableName))
	if err != nil {
		return err
	}

	defer func() {
		_ = statement.Close()
	}()

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		spans := td.ResourceSpans().At(i)
		res := spans.Resource()
		var serviceName string
		if v, ok := res.Attributes().Get(conventions.AttributeServiceName); ok {
			serviceName = v.Str()
		}
		for j := 0; j < spans.ScopeSpans().Len(); j++ {
			rs := spans.ScopeSpans().At(j).Spans()
			for k := 0; k < rs.Len(); k++ {
				r := rs.At(k)
				tagKeys, tagValues := attributesToArrays(r.Attributes())
				_, err = statement.ExecContext(ctx,
					r.StartTimestamp().AsTime(),
					traceIDToHexOrEmptyString(r.TraceID()),
					spanIDToHexOrEmptyString(r.SpanID()),
					spanIDToHexOrEmptyString(r.ParentSpanID()),
					r.Name(),
					serviceName,
					tagKeys,
					tagValues,
					uint64(r.EndTimestamp().AsTime().Sub(r.StartTimestamp().AsTime()).Nanoseconds()),
				)
				if err != nil {
					return fmt.Errorf("exec context: %w", err)
				}
			}
		}
	}

	return nil
}

func attributesToArrays(attributes pcommon.Map) ([]string, []string) {
	keys := make([]string, 0)
	values := make([]string, 0)

	attributes.Range(func(k string, v pcommon.Value) bool {
		keys = append(keys, k)
		values = append(values, v.AsString())
		return true
	})
	return keys, values
}

// spanIDToHexOrEmptyString returns a hex string from SpanID.
// An empty string is returned, if SpanID is empty.
func spanIDToHexOrEmptyString(id pcommon.SpanID) string {
	if id.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(id[:])
}

// traceIDToHexOrEmptyString returns a hex string from TraceID.
// An empty string is returned, if TraceID is empty.
func traceIDToHexOrEmptyString(id pcommon.TraceID) string {
	if id.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(id[:])
}

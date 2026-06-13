// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func (r *TraceReader) FindTraceSummaries(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		var (
			args   []any
			filter []string
		)

		filter = append(filter, "start_time >= ?")
		args = append(args, query.StartTimeMin)

		filter = append(filter, "start_time <= ?")
		args = append(args, query.StartTimeMax)

		if query.ServiceName != "" {
			// changed: apply service filter directly in native summary query
			filter = append(filter, "service_name = ?")
			args = append(args, query.ServiceName)
		}

		if query.OperationName != "" {
			// changed: apply operation filter directly in native summary query
			filter = append(filter, "operation_name = ?")
			args = append(args, query.OperationName)
		}

		limit := query.NumTraces
		if limit <= 0 {
			limit = 100
		}

		args = append(args, limit)

		sql := fmt.Sprintf(`
			SELECT
				trace_id,
				min(start_time) AS min_start_time,
				max(start_time + toIntervalNanosecond(duration)) AS max_end_time,
				count() AS span_count,
				countIf(status_code = 2) AS error_span_count,

				-- changed: derive root service directly from earliest span
				argMin(service_name, start_time) AS root_service_name,

				-- changed: derive root operation directly from earliest span
				argMin(operation_name, start_time) AS root_operation_name

			FROM spans
			WHERE %s
			GROUP BY trace_id
			ORDER BY min_start_time DESC
			LIMIT ?
		`, strings.Join(filter, " AND "))

		rows, err := r.db.Query(ctx, sql, args...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		summaries := make([]tracestore.TraceSummary, 0, limit)

		for rows.Next() {
			var (
				summary     tracestore.TraceSummary
				minStart    time.Time
				maxEnd      time.Time
				rootSvc     string
				rootOp      string
				spanCount   uint64
				errorCounts uint64
			)

			err := rows.Scan(
				&summary.TraceID,
				&minStart,
				&maxEnd,
				&spanCount,
				&errorCounts,
				&rootSvc,
				&rootOp,
			)
			if err != nil {
				yield(nil, err)
				return
			}

			summary.MinStartTime = minStart
			summary.MaxEndTime = maxEnd
			summary.RootServiceName = rootSvc
			summary.RootOperationName = rootOp
			summary.SpanCount = int(spanCount)
			summary.ErrorSpanCount = int(errorCounts)

			// changed: skip orphan reconstruction to avoid full trace materialization
			summary.OrphanSpanCount = 0

			summaries = append(summaries, summary)
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
			return
		}

		yield(summaries, nil)
	}
}
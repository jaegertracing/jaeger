WITH
selected AS (
    SELECT trace_id FROM (
		SELECT
		    l.trace_id,
		    min(t.start) AS start,
		    max(t.end) AS end
		FROM (
			SELECT DISTINCT
			    s.trace_id
			FROM spans s
			WHERE 1=1
				AND s.service_name = ?
				AND s.start_time >= ?
				AND s.start_time <= ?
			ORDER BY s.trace_id
			LIMIT ?
		) l
		LEFT JOIN trace_id_timestamps t ON l.trace_id = t.trace_id
		GROUP BY l.trace_id
    )
),
agg AS (
    SELECT
        s.trace_id AS trace_id,
        min(s.start_time) AS min_start,
        max(s.start_time + toIntervalNanosecond(s.duration)) AS max_end,
        count() AS span_count,
        countIf(s.status_code = 'Error') AS error_span_count,
        argMinIf((s.service_name, s.name), (s.start_time, s.id), s.parent_span_id = '') AS root,
        sumMap([s.service_name], [toUInt64(1)], [toUInt64(s.status_code = 'Error')]) AS svc_stats
    FROM spans s
    WHERE s.trace_id IN (SELECT trace_id FROM selected)
    GROUP BY s.trace_id
)
SELECT
    agg.trace_id AS trace_id,
    agg.min_start AS min_start,
    agg.max_end AS max_end,
    agg.span_count AS span_count,
    agg.error_span_count AS error_span_count,
    agg.root.1 AS root_service,
    agg.root.2 AS root_operation,
    agg.svc_stats.1 AS svc_names,
    agg.svc_stats.2 AS svc_span_counts,
    agg.svc_stats.3 AS svc_error_counts,
    toUInt64(ifNull(orph.orphan_count, 0)) AS orphan_count
FROM agg
LEFT JOIN (
    SELECT
        s.trace_id AS trace_id,
        count() AS orphan_count
    FROM spans s
    LEFT ANTI JOIN spans p ON s.trace_id = p.trace_id AND s.parent_span_id = p.id
    WHERE s.parent_span_id != '' AND s.trace_id IN (SELECT trace_id FROM selected)
    GROUP BY s.trace_id
) AS orph ON agg.trace_id = orph.trace_id

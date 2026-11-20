CREATE MATERIALIZED VIEW IF NOT EXISTS trace_id_timestamps_mv
TO trace_id_timestamps{{ .LocalTableSuffix }}
AS
SELECT
    trace_id,
    min(start_time) AS start,
    max(start_time) AS end
FROM spans{{ .LocalTableSuffix }}
GROUP BY trace_id;

CREATE MATERIALIZED VIEW IF NOT EXISTS trace_id_timestamps_mv
TO trace_id_timestamps
AS
SELECT
    trace_id,
    min(start_time) AS start,
    max(addNanoseconds(start_time, duration)) AS end
FROM spans
GROUP BY trace_id;
